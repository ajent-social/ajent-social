import argparse
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

SPEC = importlib.util.spec_from_file_location("codebuild_image", Path(__file__).resolve().parents[1] / "codebuild-image.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
REVISION = "a" * 40
DIGEST = "sha256:" + "b" * 64
REPOSITORY = "123456789012.dkr.ecr.us-west-2.amazonaws.com/application-images-test"


class ImageBuildTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.directory = Path(self.temporary.name)
        self.args = argparse.Namespace(project="release-image", bucket="release-source", repository=REPOSITORY,
                                       revision=REVISION, receipt=str(self.directory / "receipt.json"),
                                       source_directory=self.directory, timeout_seconds=60)
        self.calls = []

    def tearDown(self):
        self.temporary.cleanup()

    def receipt(self):
        return json.loads(Path(self.args.receipt).read_text())

    def archive(self, directory, revision, destination):
        destination.write_bytes(b"archive")
        return "test-checksum"

    def aws(self, *arguments):
        self.calls.append(arguments)
        command = arguments[:2]
        if command == ("s3api", "put-object"):
            return {"VersionId": "version-1"}
        if command == ("codebuild", "start-build"):
            # The durable request receipt precedes any possible remote effect.
            self.assertEqual(self.receipt()["status"], "START_REQUESTED")
            return {"build": {"id": "release-image:one"}}
        if command == ("codebuild", "batch-get-builds"):
            return {"builds": [{"id": "release-image:one", "buildStatus": "SUCCEEDED"}]}
        if command == ("ecr", "describe-images"):
            return {"imageDetails": [{"imageDigest": DIGEST}]}
        if command == ("codebuild", "stop-build"):
            return {"build": {"id": "release-image:one"}}
        self.fail(f"Unexpected command {command}")

    def run_build(self, aws=None, **kwargs):
        with patch.object(MODULE, "archive_source", self.archive):
            return MODULE.build_image(self.args, aws_call=aws or self.aws, **kwargs)

    def test_pins_versioned_archive_and_reads_digest_only_after_success(self):
        self.assertEqual(self.run_build(), DIGEST)
        start = next(call for call in self.calls if call[:2] == ("codebuild", "start-build"))
        self.assertIn("--source-type-override", start)
        self.assertEqual(start[start.index("--source-version") + 1], "version-1")
        overrides = json.loads(start[start.index("--environment-variables-override") + 1])
        self.assertEqual(overrides[0]["value"], REVISION)
        receipt = self.receipt()
        self.assertEqual(receipt["status"], "PUBLISHED_VERIFIED")
        self.assertEqual(receipt["image_digest"], DIGEST)
        self.assertEqual(os.stat(self.args.receipt).st_mode & 0o777, 0o600)

    def test_unversioned_source_bucket_never_starts_build(self):
        def call(*arguments):
            if arguments[:2] == ("s3api", "put-object"):
                self.calls.append(arguments)
                return {}
            return self.aws(*arguments)
        with self.assertRaisesRegex(RuntimeError, "versioning enabled"):
            self.run_build(call)
        self.assertEqual(self.receipt()["status"], "PREPARATION_FAILED")
        self.assertFalse(any(call[:2] == ("codebuild", "start-build") for call in self.calls))

    def test_start_network_failure_is_unknown_and_never_retried(self):
        def call(*arguments):
            if arguments[:2] == ("codebuild", "start-build"):
                self.calls.append(arguments)
                raise subprocess.TimeoutExpired("aws", 90)
            return self.aws(*arguments)
        with self.assertRaises(subprocess.TimeoutExpired):
            self.run_build(call)
        self.assertEqual(self.receipt()["status"], "UNKNOWN_START_OUTCOME")
        self.assertEqual(sum(call[:2] == ("codebuild", "start-build") for call in self.calls), 1)
        with self.assertRaisesRegex(ValueError, "Receipt already exists"):
            self.run_build()

    def test_failed_build_never_reads_image_digest(self):
        def call(*arguments):
            if arguments[:2] == ("codebuild", "batch-get-builds"):
                self.calls.append(arguments)
                return {"builds": [{"id": "release-image:one", "buildStatus": "FAILED"}]}
            return self.aws(*arguments)
        with self.assertRaisesRegex(RuntimeError, "ended FAILED"):
            self.run_build(call)
        self.assertEqual(self.receipt()["status"], "FAILED")
        self.assertFalse(any(call[0] == "ecr" for call in self.calls))

    def test_timeout_requests_stop_and_preserves_build_custody(self):
        times = iter([0, 0, 61])
        with self.assertRaises(TimeoutError):
            self.run_build(monotonic=lambda: next(times))
        self.assertEqual(self.receipt()["build_id"], "release-image:one")
        self.assertEqual(self.receipt()["status"], "STOP_REQUESTED")
        self.assertTrue(any(call[:2] == ("codebuild", "stop-build") for call in self.calls))

    def test_interrupt_requests_stop(self):
        def call(*arguments):
            if arguments[:2] == ("codebuild", "batch-get-builds"):
                raise KeyboardInterrupt
            return self.aws(*arguments)
        with self.assertRaises(KeyboardInterrupt):
            self.run_build(call)
        self.assertEqual(self.receipt()["status"], "STOP_REQUESTED")

    def test_poll_failure_and_failed_stop_are_unknown(self):
        def call(*arguments):
            if arguments[:2] in (("codebuild", "batch-get-builds"), ("codebuild", "stop-build")):
                raise subprocess.TimeoutExpired("aws", 90)
            return self.aws(*arguments)
        with self.assertRaises(subprocess.TimeoutExpired):
            self.run_build(call)
        self.assertEqual(self.receipt()["status"], "UNKNOWN_BUILD_STATE")

    def test_malformed_success_digest_is_not_verified(self):
        def call(*arguments):
            if arguments[:2] == ("ecr", "describe-images"):
                return {"imageDetails": [{"imageDigest": "latest"}]}
            return self.aws(*arguments)
        with self.assertRaisesRegex(RuntimeError, "immutable ECR digest"):
            self.run_build(call)
        self.assertEqual(self.receipt()["status"], "SUCCEEDED")
        self.assertNotIn("image_digest", self.receipt())

    def test_shell_rejects_marker_mismatch_before_cloud_calls(self):
        (self.directory / ".ajent-release-revision").write_text("wrong")
        environment = dict(os.environ, AJENT_REVISION=REVISION, AJENT_REPOSITORY=REPOSITORY,
                           AWS_REGION="us-west-2")
        result = subprocess.run(["bash", str(Path(MODULE.__file__).parent / "aws-release-build.sh")],
                                cwd=self.directory, env=environment, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Source revision marker mismatch", result.stderr)

    def test_shell_rejects_existing_tag_and_permission_error(self):
        (self.directory / ".ajent-release-revision").write_text(REVISION)
        bin_directory = self.directory / "bin"
        bin_directory.mkdir()
        for name, content in {
            "uname": "#!/bin/sh\necho aarch64\n",
            "aws": "#!/bin/sh\nexit 0\n",
            "docker": "#!/bin/sh\necho docker-must-not-run >&2\nexit 99\n",
        }.items():
            executable = bin_directory / name
            executable.write_text(content)
            executable.chmod(0o700)
        environment = dict(os.environ, AJENT_REVISION=REVISION, AJENT_REPOSITORY=REPOSITORY,
                           AWS_REGION="us-west-2", PATH=str(bin_directory) + os.pathsep + os.environ["PATH"])
        script = str(Path(MODULE.__file__).parent / "aws-release-build.sh")
        existing = subprocess.run(["bash", script], cwd=self.directory, env=environment,
                                  capture_output=True, text=True)
        self.assertNotEqual(existing.returncode, 0)
        self.assertIn("Revision tag already exists", existing.stderr)
        self.assertNotIn("docker-must-not-run", existing.stderr)
        (bin_directory / "aws").write_text("#!/bin/sh\necho '(AccessDeniedException)' >&2\nexit 1\n")
        denied = subprocess.run(["bash", script], cwd=self.directory, env=environment,
                                capture_output=True, text=True)
        self.assertNotEqual(denied.returncode, 0)
        self.assertIn("AccessDeniedException", denied.stderr)
        self.assertNotIn("docker-must-not-run", denied.stderr)

    def test_source_archive_excludes_dirty_and_untracked_files(self):
        repo = self.directory / "repo"
        repo.mkdir()
        subprocess.run(["git", "init", "-q", str(repo)], check=True)
        (repo / "scripts").mkdir()
        (repo / "scripts/aws-release-build.sh").write_text("reviewed\n")
        subprocess.run(["git", "-C", str(repo), "add", "."], check=True)
        subprocess.run(["git", "-C", str(repo), "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "source"], check=True)
        revision = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
        (repo / "scripts/aws-release-build.sh").write_text("dirty\n")
        (repo / "credentials.env").write_text("untracked private contents")
        destination = self.directory / "source.zip"
        MODULE.archive_source(repo, revision, destination)
        with zipfile.ZipFile(destination) as archive:
            self.assertNotIn("credentials.env", archive.namelist())
            self.assertEqual(archive.read("scripts/aws-release-build.sh"), b"reviewed\n")
            self.assertEqual(archive.read(".ajent-release-revision"), (revision + "\n").encode())


if __name__ == "__main__":
    unittest.main()
