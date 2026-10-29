#!/usr/bin/env python3
"""Publish one exact source archive through CodeBuild; never retry uncertain starts."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time
import uuid
import zipfile

TERMINAL = {"SUCCEEDED", "FAILED", "FAULT", "STOPPED", "TIMED_OUT"}


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def aws(*args):
    result = subprocess.run(
        ["aws", "--cli-connect-timeout", "10", "--cli-read-timeout", "60", *args, "--output", "json"],
        check=True, capture_output=True, text=True, timeout=90,
    )
    return json.loads(result.stdout) if result.stdout.strip() else {}


def save_receipt(path, receipt):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=path.parent, delete=False) as file:
        json.dump(receipt, file, indent=2)
        file.write("\n")
        temporary = file.name
    os.chmod(temporary, 0o600)
    os.replace(temporary, path)


def archive_source(directory, revision, destination):
    resolved = subprocess.check_output(
        ["git", "-C", str(directory), "rev-parse", revision + "^{commit}"], text=True,
    ).strip()
    if resolved != revision:
        raise ValueError("Revision must identify the exact full commit")
    subprocess.run(
        ["git", "-C", str(directory), "archive", "--format=zip", "--output", str(destination), revision],
        check=True, capture_output=True,
    )
    with zipfile.ZipFile(destination, "a") as archive:
        if ".ajent-release-revision" in archive.namelist():
            raise ValueError("Source must not already contain the release revision marker")
        if "scripts/aws-release-build.sh" not in archive.namelist():
            raise ValueError("Commit lacks the reviewed CodeBuild entry point")
        archive.writestr(".ajent-release-revision", revision + "\n")
    return hashlib.sha256(destination.read_bytes()).hexdigest()


def build_image(args, *, aws_call=aws, monotonic=time.monotonic, sleep=time.sleep):
    if not re.fullmatch(r"[a-f0-9]{40}", args.revision):
        raise ValueError("--revision must be a full lowercase commit SHA")
    if not re.fullmatch(r"[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com/[a-z0-9][a-z0-9._/-]*", args.repository):
        raise ValueError("--repository must be an ECR repository URL")
    if not 60 <= args.timeout_seconds <= 10800:
        raise ValueError("Timeout must be between 60 and 10800 seconds")
    receipt_path = Path(args.receipt)
    if receipt_path.exists():
        raise ValueError("Receipt already exists; reconcile that attempt instead of starting another")
    attempt = uuid.uuid4().hex
    receipt = {"attempt": attempt, "revision": args.revision, "project": args.project,
               "repository": args.repository, "source_bucket": args.bucket,
               "source_key": f"sources/{args.revision}/{attempt}.zip", "created_at": utc_now(),
               "status": "PREPARING", "build_id": None}
    receipt_path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    descriptor = os.open(receipt_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w") as file:
        json.dump(receipt, file, indent=2)
        file.write("\n")
    deadline = monotonic() + args.timeout_seconds
    try:
        with tempfile.TemporaryDirectory(prefix="ajent-codebuild-") as temporary:
            archive = Path(temporary) / "source.zip"
            receipt["archive_sha256"] = archive_source(args.source_directory, args.revision, archive)
            # s3api put-object returns a compact JSON response and avoids progress output.
            uploaded = aws_call("s3api", "put-object", "--bucket", args.bucket,
                                "--key", receipt["source_key"], "--body", str(archive),
                                "--checksum-algorithm", "SHA256")
            receipt["source_version_id"] = uploaded.get("VersionId")
            if not receipt["source_version_id"] or receipt["source_version_id"] == "null":
                raise RuntimeError("Source bucket must have versioning enabled; no build was started")
        receipt["status"] = "START_REQUESTED"
        receipt["start_requested_at"] = utc_now()
        save_receipt(receipt_path, receipt)
        if monotonic() >= deadline:
            receipt["status"] = "DEADLINE_BEFORE_START"
            raise TimeoutError("Deadline elapsed before starting CodeBuild")
        overrides = json.dumps([
            {"name": "AJENT_REVISION", "value": args.revision, "type": "PLAINTEXT"},
            {"name": "AJENT_REPOSITORY", "value": args.repository, "type": "PLAINTEXT"},
        ])
        start_args = ["codebuild", "start-build", "--project-name", args.project,
                      "--source-type-override", "S3", "--source-location-override",
                      f"{args.bucket}/{receipt['source_key']}", "--idempotency-token", attempt,
                      "--environment-variables-override", overrides]
        if receipt["source_version_id"]:
            start_args += ["--source-version", receipt["source_version_id"]]
        result = aws_call(*start_args)
        build_id = result["build"]["id"]
        if not isinstance(build_id, str) or not build_id.startswith(args.project + ":"):
            raise RuntimeError("Start response did not identify this project; reconcile before retry")
        receipt["build_id"] = build_id
        receipt["status"] = "IN_PROGRESS"
        save_receipt(receipt_path, receipt)
        print(f"CodeBuild {receipt['build_id']} started; receipt: {receipt_path}", file=sys.stderr)
        while True:
            if monotonic() >= deadline:
                raise TimeoutError("CodeBuild wait deadline exceeded")
            result = aws_call("codebuild", "batch-get-builds", "--ids", receipt["build_id"])
            builds = result.get("builds", [])
            if len(builds) != 1 or builds[0].get("id") != receipt["build_id"]:
                raise RuntimeError("CodeBuild returned no unique matching build; reconcile before retry")
            build = builds[0]
            status = build["buildStatus"]
            if status != receipt["status"]:
                receipt["status"] = status
                save_receipt(receipt_path, receipt)
                print(f"CodeBuild status: {status}", file=sys.stderr)
            if status in TERMINAL:
                receipt["completed_at"] = utc_now()
                receipt["logs"] = {key: build.get("logs", {}).get(key) for key in ("groupName", "streamName", "deepLink")}
                save_receipt(receipt_path, receipt)
                if status != "SUCCEEDED":
                    raise RuntimeError(f"CodeBuild ended {status}; inspect the recorded build logs")
                break
            sleep(min(15, max(0, deadline - monotonic())))
        images = aws_call("ecr", "describe-images", "--repository-name", args.repository.split("/", 1)[1],
                          "--image-ids", f"imageTag={args.revision}").get("imageDetails", [])
        if len(images) != 1 or not re.fullmatch(r"sha256:[a-f0-9]{64}", images[0].get("imageDigest", "")):
            raise RuntimeError("Successful build has no unique immutable ECR digest")
        receipt["image_digest"] = images[0]["imageDigest"]
        receipt["status"] = "PUBLISHED_VERIFIED"
        save_receipt(receipt_path, receipt)
        return receipt["image_digest"]
    except BaseException as error:
        previous = receipt["status"]
        if receipt["build_id"] and previous not in TERMINAL | {"PUBLISHED_VERIFIED"}:
            receipt["status"] = "STOP_REQUESTED"
            try:
                aws_call("codebuild", "stop-build", "--id", receipt["build_id"])
                receipt["stop_requested_at"] = utc_now()
            except BaseException:
                receipt["status"] = "UNKNOWN_BUILD_STATE"
        elif previous == "START_REQUESTED":
            receipt["status"] = "UNKNOWN_START_OUTCOME"
        elif previous == "PREPARING":
            receipt["status"] = "PREPARATION_FAILED"
        receipt["error_type"] = type(error).__name__
        save_receipt(receipt_path, receipt)
        raise


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("project", "bucket", "repository", "revision", "receipt"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--source-directory", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--timeout-seconds", type=int, default=3600)
    return parser.parse_args(argv)


def main():
    def terminate(_signum, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, terminate)
    try:
        print(build_image(parse_args()))
    except KeyboardInterrupt:
        print("Interrupted; inspect receipt and reconcile build state before retry.", file=sys.stderr)
        return 130
    except Exception as error:
        # AWS stderr may contain request details. The private receipt and AWS
        # build ID are the diagnostic record; never dump captured API output.
        print(f"Image publication failed ({type(error).__name__}): {error if not isinstance(error, subprocess.SubprocessError) else 'AWS or git command failed; inspect receipt.'}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
