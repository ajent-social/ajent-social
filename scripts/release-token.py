#!/usr/bin/env python3
"""Install/revoke a three-hour personal Pulumi release token without logging it."""
import argparse
import datetime
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

API = "https://api.pulumi.com"
STACK = "david-sire-run/ajent-release-executor/production"
TOKEN_TYPE = "github:index/actionsSecret:ActionsSecret"
TOKEN_URN = "urn:pulumi:production::ajent-release-executor::" + TOKEN_TYPE + "::pulumiToken"
TTL_SECONDS = 10800


class TokenError(RuntimeError):
    pass


class APIError(TokenError):
    def __init__(self, status):
        self.status = status
        super().__init__(f"Pulumi API returned HTTP {status}")


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def save_receipt(path, record):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=path.parent, delete=False) as output:
        json.dump(record, output, indent=2)
        output.write("\n")
        temporary = output.name
    os.chmod(temporary, 0o600)
    os.replace(temporary, path)


def reserve_receipt(path, record):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w") as output:
        json.dump(record, output, indent=2)
        output.write("\n")


def owner_auth(environment=None, credentials_path=None):
    environment = os.environ if environment is None else environment
    if environment.get("PULUMI_ACCESS_TOKEN"):
        return environment["PULUMI_ACCESS_TOKEN"]
    path = credentials_path or Path.home() / ".pulumi/credentials.json"
    try:
        credentials = json.loads(Path(path).read_text())
    except (OSError, ValueError):
        raise TokenError("No readable Pulumi owner authentication for api.pulumi.com") from None
    access_tokens = credentials.get("accessTokens", {})
    token = access_tokens.get(API) if isinstance(access_tokens, dict) else None
    if not token and credentials.get("current") == API:
        token = credentials.get("currentAccessToken")
    if not token:
        accounts = credentials.get("accounts", {})
        account = accounts.get(API, {}) if isinstance(accounts, dict) else {}
        token = account.get("accessToken") if isinstance(account, dict) else None
    if not isinstance(token, str) or not token:
        raise TokenError("No Pulumi owner authentication for api.pulumi.com")
    return token


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        raise APIError(code)


def api_request(method, path, owner_token, body=None):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(API + path, data=data, method=method,
                                     headers={"Authorization": "token " + owner_token,
                                              "Content-Type": "application/json"})
    try:
        with urllib.request.build_opener(NoRedirect).open(request, timeout=30) as response:
            content = response.read()
    except urllib.error.HTTPError as error:
        # Never print remote response bodies or Authorization headers.
        raise APIError(error.code) from None
    return json.loads(content) if content else {}


def verify_preview(events):
    """Allow only plaintextValue on the one existing GitHub secret to change."""
    seen_target = False
    summary = None
    for line in events.splitlines():
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except ValueError:
            raise TokenError("Preview was not streaming JSON") from None
        if "diagnosticEvent" in event and event["diagnosticEvent"].get("severity") == "error":
            raise TokenError("Preview contains an error diagnostic")
        metadata = event.get("resourcePreEvent", {}).get("metadata")
        if metadata:
            operation, urn = metadata.get("op"), metadata.get("urn", "")
            if operation == "same":
                if urn == TOKEN_URN:
                    old = metadata.get("old") or {}
                    if not old.get("id"):
                        raise TokenError("Secret must already exist")
                    inputs = old.get("inputs") or {}
                    if inputs.get("repository") != "production-release" or inputs.get("secretName") != "PULUMI_ACCESS_TOKEN":
                        raise TokenError("Existing secret identity does not match release custody")
                    seen_target = True
                continue
            if operation == "update" and metadata.get("type") == "pulumi:pulumi:Stack":
                continue
            if operation != "update" or urn != TOKEN_URN or metadata.get("type") != TOKEN_TYPE:
                raise TokenError("Preview changes a resource outside the existing release secret")
            old = metadata.get("old") or {}
            new = metadata.get("new") or {}
            if not old.get("id") or old.get("id") != new.get("id"):
                raise TokenError("Preview must update the same existing secret")
            old_inputs, new_inputs = dict(old.get("inputs") or {}), dict(new.get("inputs") or {})
            if "plaintextValue" not in old_inputs or "plaintextValue" not in new_inputs:
                raise TokenError("Secret preview omitted plaintextValue inputs")
            old_inputs.pop("plaintextValue")
            new_inputs.pop("plaintextValue")
            if old_inputs != new_inputs:
                raise TokenError("Preview changes secret inputs other than plaintextValue")
            if old_inputs.get("repository") != "production-release" or old_inputs.get("secretName") != "PULUMI_ACCESS_TOKEN":
                raise TokenError("Preview secret identity does not match release custody")
            if any(key != "plaintextValue" for key in metadata.get("diffs", [])):
                raise TokenError("Preview changes properties outside plaintextValue")
            if any(key != "plaintextValue" for key in (metadata.get("detailedDiff") or {})):
                raise TokenError("Preview changes detailed properties outside plaintextValue")
            seen_target = True
        if "summaryEvent" in event:
            summary = event["summaryEvent"]
    if not seen_target or summary is None:
        raise TokenError("Preview lacks the existing secret or final summary")
    changes = summary.get("resourceChanges", {})
    if any(value and key not in {"same", "update"} for key, value in changes.items()):
        raise TokenError("Preview summary contains a disallowed change")
    if changes.get("update", 0) > 2:
        raise TokenError("Preview summary contains too many updates")


def run_command(command, *, environment, input_text=None, timeout=600):
    try:
        result = subprocess.run(command, input=input_text, text=True, capture_output=True,
                                env=environment, timeout=timeout, check=True)
    except subprocess.SubprocessError:
        raise TokenError("Local release-token command failed; captured output was withheld") from None
    return result.stdout


def install_secret(value, args, owner_token, *, command=run_command):
    environment = dict(os.environ, PULUMI_ACCESS_TOKEN=owner_token,
                       PULUMI_ENABLE_STREAMING_JSON_PREVIEW="true", PULUMI_EXPERIMENTAL="true")
    if not environment.get("GITHUB_TOKEN"):
        environment["GITHUB_TOKEN"] = command(["gh", "auth", "token"], environment=environment).strip()
    if not environment["GITHUB_TOKEN"]:
        raise TokenError("No GitHub authentication available for the IaC provider")
    base = ["pulumi", "-C", str(args.directory)]
    command(base + ["config", "set", "pulumiAccessToken", "--secret", "--stack", args.stack,
                    "--non-interactive"], environment=environment, input_text=value + "\n", timeout=90)
    with tempfile.TemporaryDirectory(prefix="ajent-token-plan-") as temporary:
        os.chmod(temporary, 0o700)
        plan = Path(temporary) / "plan.json"
        output = command(base + ["preview", "--stack", args.stack, "--refresh=false", "--json",
                                 "--save-plan", str(plan), "--non-interactive"], environment=environment)
        if value and value != "disabled" and value in output:
            raise TokenError("Preview exposed a secret; output was withheld")
        verify_preview(output)
        command(base + ["up", "--stack", args.stack, "--refresh=false", "--plan", str(plan),
                        "--yes", "--non-interactive"], environment=environment)


def token_id(record):
    identifier = record.get("token_id")
    if not isinstance(identifier, str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", identifier):
        raise TokenError("Receipt lacks a valid exact token id; reconcile the recorded request manually")
    return identifier


def revoke_exact(record, receipt, owner_token, *, request=api_request):
    identifier = token_id(record)
    record["revocation_requested_at"] = utc_now()
    record["status"] = "REVOCATION_REQUESTED"
    save_receipt(receipt, record)
    try:
        request("DELETE", "/api/user/tokens/" + urllib.parse.quote(identifier, safe=""), owner_token)
        record["revocation_outcome"] = "deleted"
    except APIError as error:
        if error.status != 404:
            record["status"] = "UNKNOWN_REVOCATION_OUTCOME"
            save_receipt(receipt, record)
            raise
        record["revocation_outcome"] = "already_absent"
    except BaseException:
        record["status"] = "UNKNOWN_REVOCATION_OUTCOME"
        save_receipt(receipt, record)
        raise
    record["revocation_verified"] = True
    record["revoked_at"] = utc_now()
    record["status"] = "REVOKED"
    save_receipt(receipt, record)


def create(args, *, request=api_request, authenticate=owner_auth, install=install_secret, now=time.time):
    owner_token = authenticate()
    record = {"stack": args.stack, "description": "ajent-release-personal-" + uuid.uuid4().hex,
              "requested_at": utc_now(), "expires": int(now()) + TTL_SECONDS,
              "token_kind": "personal", "status": "CREATE_REQUESTED"}
    reserve_receipt(args.receipt, record)
    try:
        response = request("POST", "/api/user/tokens", owner_token,
                           {"description": record["description"], "expires": record["expires"]})
        if isinstance(response.get("id"), str):
            record["token_id"] = response["id"]
            record["status"] = "MINTED"
            save_receipt(args.receipt, record)
        token_id(record)
        value = response.get("tokenValue")
        if not isinstance(value, str) or not value:
            raise TokenError("Mint response lacked tokenValue; revoking the recorded token")
        install(value, args, owner_token)
        record["status"] = "INSTALLED"
        record["installed_at"] = utc_now()
        save_receipt(args.receipt, record)
    except BaseException as error:
        if record.get("token_id"):
            try:
                revoke_exact(record, args.receipt, owner_token, request=request)
            except BaseException:
                record["cleanup_revocation"] = "unknown"
            try:
                install("disabled", args, owner_token)
                record["secret_disabled"] = True
            except BaseException:
                record["secret_disabled"] = False
            record["status"] = "CREATE_FAILED_REVOKED" if record.get("revocation_verified") else "CREATE_FAILED_REVOCATION_UNKNOWN"
        else:
            record["status"] = "UNKNOWN_CREATE_OUTCOME"
        record["error_type"] = type(error).__name__
        save_receipt(args.receipt, record)
        raise
    return record


def revoke(args, *, request=api_request, authenticate=owner_auth, install=install_secret):
    record = json.loads(args.receipt.read_text())
    if record.get("stack") != args.stack:
        raise TokenError("Receipt stack mismatch")
    owner_token = authenticate()
    revoke_exact(record, args.receipt, owner_token, request=request)
    try:
        install("disabled", args, owner_token)
        record["secret_disabled"] = True
        record["status"] = "REVOKED_AND_SECRET_DISABLED"
    except BaseException:
        record["secret_disabled"] = False
        record["status"] = "REVOKED_SECRET_DISABLE_FAILED"
        save_receipt(args.receipt, record)
        raise
    save_receipt(args.receipt, record)
    return record


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument("--create", action="store_true")
    action.add_argument("--revoke", action="store_true")
    parser.add_argument("--receipt", type=Path, required=True)
    parser.add_argument("--stack", choices=[STACK], required=True)
    parser.add_argument("--directory", type=Path, default=Path(__file__).resolve().parents[1] / "infra/release-executor")
    return parser.parse_args(argv)


def main():
    def terminate(_signum, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, terminate)
    args = parse_args()
    try:
        record = create(args) if args.create else revoke(args)
        print(json.dumps({"status": record["status"], "receipt": str(args.receipt)}))
        return 0
    except BaseException as error:
        print(f"Release token operation failed ({type(error).__name__}); reconcile the private receipt before another create.", file=sys.stderr)
        return 130 if isinstance(error, KeyboardInterrupt) else 1


if __name__ == "__main__":
    sys.exit(main())
