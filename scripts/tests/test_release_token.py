import argparse
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("release_token", Path(__file__).resolve().parents[1] / "release-token.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def secret_event():
    old = {"id": "production-release:PULUMI_ACCESS_TOKEN", "urn": MODULE.TOKEN_URN,
           "inputs": {"repository": "production-release", "secretName": "PULUMI_ACCESS_TOKEN", "plaintextValue": "[secret]"}}
    new = copy.deepcopy(old)
    new["id"] = ""  # Actual Pulumi 3.226 planned-update state shape.
    return {"resourcePreEvent": {"metadata": {"op": "update", "urn": MODULE.TOKEN_URN,
            "type": MODULE.TOKEN_TYPE, "old": old, "new": new,
            "diffs": ["plaintextValue"], "detailedDiff": {"plaintextValue": {"kind": "update"}}}}}


def preview(*events):
    return "\n".join(json.dumps(event) for event in [*events, {"summaryEvent": {"resourceChanges": {"same": 20, "update": 1}}}])


class TokenTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.directory = Path(self.temporary.name)
        self.args = argparse.Namespace(receipt=self.directory / "receipt.json", stack=MODULE.STACK,
                                       directory=self.directory)
        self.calls = []
        self.installs = []

    def tearDown(self):
        self.temporary.cleanup()

    def receipt(self):
        return json.loads(self.args.receipt.read_text())

    def request(self, method, path, auth, body=None):
        self.calls.append((method, path, auth, body))
        if method == "POST":
            self.assertEqual(self.receipt()["status"], "CREATE_REQUESTED")
            return {"id": "exact-id-123", "tokenValue": "minted-secret-never-log"}
        if method == "DELETE":
            self.assertEqual(path, "/api/user/tokens/exact-id-123")
            self.assertEqual(auth, "original-owner-auth")
            return {}
        self.fail("Unexpected token API action")

    def install(self, value, args, owner):
        self.installs.append(value)
        self.assertEqual(owner, "original-owner-auth")
        self.assertEqual(self.receipt()["token_id"], "exact-id-123")

    def create(self, **overrides):
        kwargs = dict(request=self.request, authenticate=lambda: "original-owner-auth",
                      install=self.install, now=lambda: 1000)
        kwargs.update(overrides)
        return MODULE.create(self.args, **kwargs)

    def test_mints_personal_token_with_exact_three_hour_expiry_and_private_receipt(self):
        record = self.create()
        self.assertEqual(record["status"], "INSTALLED")
        self.assertEqual(self.calls[0][3]["expires"], 11800)
        self.assertEqual(record["token_kind"], "personal")
        self.assertEqual(self.installs, ["minted-secret-never-log"])
        contents = self.args.receipt.read_text()
        self.assertNotIn("minted-secret-never-log", contents)
        self.assertNotIn("original-owner-auth", contents)
        self.assertEqual(os.stat(self.args.receipt).st_mode & 0o777, 0o600)

    def test_uncertain_mint_is_durable_and_not_retried(self):
        count = []
        def timeout(*args):
            count.append(args)
            raise TimeoutError("response lost")
        with self.assertRaises(TimeoutError):
            self.create(request=timeout)
        self.assertEqual(len(count), 1)
        self.assertEqual(self.receipt()["status"], "UNKNOWN_CREATE_OUTCOME")
        self.assertNotIn("token_id", self.receipt())
        with self.assertRaises(FileExistsError):
            self.create(request=timeout)
        self.assertEqual(len(count), 1)

    def test_install_failure_revokes_only_new_exact_id_then_disables(self):
        def install(value, args, auth):
            self.installs.append(value)
            if value != "disabled":
                raise MODULE.TokenError("preview failed")
        with self.assertRaises(MODULE.TokenError):
            self.create(install=install)
        self.assertEqual(self.calls[1][:2], ("DELETE", "/api/user/tokens/exact-id-123"))
        self.assertEqual(self.installs, ["minted-secret-never-log", "disabled"])
        self.assertTrue(self.receipt()["revocation_verified"])
        self.assertTrue(self.receipt()["secret_disabled"])
        self.assertEqual(self.receipt()["status"], "CREATE_FAILED_REVOKED")

    def test_unknown_revocation_is_not_reported_as_success(self):
        self.create()
        def timeout(*args):
            raise TimeoutError
        with self.assertRaises(TimeoutError):
            MODULE.revoke(self.args, request=timeout, authenticate=lambda: "original-owner-auth", install=self.install)
        self.assertEqual(self.receipt()["status"], "UNKNOWN_REVOCATION_OUTCOME")
        self.assertFalse(self.receipt().get("revocation_verified", False))

    def test_delete_404_is_already_revoked_then_disables_secret(self):
        self.create()
        self.installs.clear()
        def absent(method, path, auth, body=None):
            self.assertEqual(path, "/api/user/tokens/exact-id-123")
            raise MODULE.APIError(404)
        record = MODULE.revoke(self.args, request=absent, authenticate=lambda: "original-owner-auth", install=self.install)
        self.assertEqual(record["status"], "REVOKED_AND_SECRET_DISABLED")
        self.assertEqual(record["revocation_outcome"], "already_absent")
        self.assertEqual(self.installs, ["disabled"])

    def test_owner_auth_reads_only_api_backend(self):
        path = self.directory / "credentials.json"
        path.write_text(json.dumps({"current": "https://other.example", "currentAccessToken": "wrong",
                                    "accessTokens": {MODULE.API: "correct", "https://other.example": "wrong"}}))
        self.assertEqual(MODULE.owner_auth({}, path), "correct")
        path.write_text(json.dumps({"current": "https://other.example", "currentAccessToken": "wrong"}))
        with self.assertRaises(MODULE.TokenError):
            MODULE.owner_auth({}, path)
        self.assertEqual(MODULE.owner_auth({"PULUMI_ACCESS_TOKEN": "env"}, path), "env")

    def test_preview_allows_only_secret_value_and_aggregate_stack_update(self):
        event = secret_event()
        stack = {"resourcePreEvent": {"metadata": {"op": "update", "type": "pulumi:pulumi:Stack", "urn": "urn:pulumi:production::ajent-release-executor::pulumi:pulumi:Stack::ajent-release-executor-production"}}}
        MODULE.verify_preview(preview(stack, event))
        for mutation in ("create", "foreign", "repository", "property", "id", "old_id", "old_urn", "new_urn", "missing_old_urn", "missing_new_urn", "detailed_property"):
            changed = copy.deepcopy(event)
            metadata = changed["resourcePreEvent"]["metadata"]
            if mutation == "create": metadata["op"] = "create"
            if mutation == "foreign": metadata["urn"] += "-other"
            if mutation == "repository": metadata["new"]["inputs"]["repository"] = "another"
            if mutation == "property": metadata["diffs"].append("secretName")
            if mutation == "id": metadata["new"]["id"] = "other"
            if mutation == "old_id": metadata["old"]["id"] = "other"
            if mutation == "old_urn": metadata["old"]["urn"] += "-other"
            if mutation == "new_urn": metadata["new"]["urn"] += "-other"
            if mutation == "missing_old_urn": metadata["old"].pop("urn")
            if mutation == "missing_new_urn": metadata["new"].pop("urn")
            if mutation == "detailed_property": metadata["detailedDiff"]["secretName"] = {"kind": "update"}
            with self.subTest(mutation=mutation), self.assertRaises(MODULE.TokenError):
                MODULE.verify_preview(preview(changed))

    def test_preview_accepts_empty_absent_or_unchanged_planned_id(self):
        for planned_id in ("", None, "production-release:PULUMI_ACCESS_TOKEN"):
            event = secret_event()
            new = event["resourcePreEvent"]["metadata"]["new"]
            if planned_id is None:
                new.pop("id")
            else:
                new["id"] = planned_id
            with self.subTest(planned_id=planned_id):
                MODULE.verify_preview(preview(event))

    def test_secret_uses_stdin_and_saved_plan_apply(self):
        commands = []
        def command(arguments, **kwargs):
            commands.append((arguments, kwargs))
            if "preview" in arguments:
                return preview(secret_event())
            return ""
        with patch.dict(os.environ, {"GITHUB_TOKEN": "github-original"}):
            MODULE.install_secret("minted-secret-never-log", self.args, "original-owner-auth", command=command)
        self.assertEqual(len(commands), 3)
        self.assertIn("pulumiAccessToken", commands[0][0])
        self.assertNotIn("pulumiToken", commands[0][0])
        self.assertEqual(commands[0][1]["input_text"], "minted-secret-never-log\n")
        for arguments, kwargs in commands:
            self.assertNotIn("minted-secret-never-log", " ".join(arguments))
        self.assertIn("--save-plan", commands[1][0])
        self.assertIn("--plan", commands[2][0])

    def test_foreign_preview_never_applies(self):
        commands = []
        def command(arguments, **kwargs):
            commands.append(arguments)
            if "preview" in arguments:
                event = secret_event()
                event["resourcePreEvent"]["metadata"]["urn"] += "-other"
                return preview(event)
            return ""
        with patch.dict(os.environ, {"GITHUB_TOKEN": "github-original"}), self.assertRaises(MODULE.TokenError):
            MODULE.install_secret("minted-secret-never-log", self.args, "original-owner-auth", command=command)
        self.assertFalse(any("up" in command for command in commands))


if __name__ == "__main__":
    unittest.main()
