import argparse
import copy
import datetime
import importlib.util
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("source_app_bootstrap", Path(__file__).parents[1] / "bootstrap-source-app.py")
bootstrap = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(bootstrap)
MANIFEST = json.loads((Path(__file__).parents[2] / "runners/release/source-app-manifest.json").read_text())


def app_data(pem):
    return {"id": 123, "client_id": "Iv1.public-client", "slug": "ajent-production-source-read",
            "html_url": "https://github.com/apps/ajent-production-source-read",
            "owner": {"login": bootstrap.ORG, "id": bootstrap.ORG_ID},
            "permissions": dict(bootstrap.PERMISSIONS), "pem": pem,
            "client_secret": "unused-oauth-secret", "webhook_secret": "unused-webhook-secret"}


def installation():
    return {"id": 456, "app_id": 123, "account": {"login": bootstrap.ORG, "id": bootstrap.ORG_ID},
            "repository_selection": "selected", "permissions": dict(bootstrap.PERMISSIONS), "suspended_at": None}


class ManifestAndCallbackTests(unittest.TestCase):
    def test_manifest_is_exact_readonly_without_webhooks_or_oauth(self):
        bootstrap.validate_manifest(MANIFEST)
        for change in ({"default_permissions": {"contents": "write", "actions": "read", "metadata": "read"}},
                       {"public": True}, {"default_events": ["push"]}, {"setup_url": "https://evil.invalid"},
                       {"request_oauth_on_install": True}):
            with self.subTest(change=change), self.assertRaises(bootstrap.BootstrapError):
                bootstrap.validate_manifest(dict(MANIFEST, **change))

    def test_callback_requires_exact_state_once_and_valid_code(self):
        with tempfile.TemporaryDirectory() as directory:
            service = bootstrap.Bootstrap(directory, MANIFEST, 3600)
            self.assertEqual(service.accept_callback({"state": ["wrong"], "code": ["a" * 40]}), 403)
            self.assertEqual(service.accept_callback({"state": [service.state], "code": ["bad"]}), 400)
            self.assertEqual(service.accept_callback({"state": [service.state, service.state], "code": ["a" * 40]}), 400)
            query = {"state": [service.state], "code": ["a" * 40]}
            self.assertEqual(service.accept_callback(query), 303)
            self.assertEqual(service.accept_callback(query), 409)
            receipt = service.path.read_text()
            self.assertNotIn(service.state, receipt)
            self.assertNotIn("a" * 40, receipt)
            self.assertEqual(os.stat(directory).st_mode & 0o777, 0o700)
            self.assertEqual(os.stat(service.path).st_mode & 0o777, 0o600)

    def test_http_callback_redirects_to_clean_page_before_worker_starts(self):
        with tempfile.TemporaryDirectory() as directory:
            service = bootstrap.Bootstrap(directory, MANIFEST, 3600)
            service.origin = "http://127.0.0.1:12345"
            handler = bootstrap.handler_for(service).__new__(bootstrap.handler_for(service))
            handler.headers = {"Host": "127.0.0.1:12345"}
            handler.path = service.callback_path + "?state=" + service.state + "&code=" + "a" * 40
            events = []
            handler.respond = lambda status, body="", location=None: events.append(("response", status, location, body))
            worker = mock.Mock()
            worker.start.side_effect = lambda: events.append(("worker_start",))
            with mock.patch.object(bootstrap.threading, "Thread", return_value=worker):
                handler.do_GET()
            self.assertEqual(events[0], ("response", 303, "/waiting", ""))
            self.assertEqual(events[1], ("worker_start",))
            self.assertNotIn("a" * 40, json.dumps(events))
            handler.path = service.form_path
            handler.do_GET()
            self.assertEqual(events[-1][1], 409)

    def test_non_ascii_state_is_rejected_without_error_or_consumption(self):
        with tempfile.TemporaryDirectory() as directory:
            service = bootstrap.Bootstrap(directory, MANIFEST, 3600)
            self.assertEqual(service.accept_callback({"state": ["é" * 43], "code": ["a" * 40]}), 403)
            self.assertFalse(service.consumed)

    def test_existing_receipt_prevents_duplicate_creation(self):
        with tempfile.TemporaryDirectory() as directory:
            bootstrap.Bootstrap(directory, MANIFEST, 3600)
            with self.assertRaises(FileExistsError):
                bootstrap.Bootstrap(directory, MANIFEST, 3600)

    def test_form_contains_random_local_redirect_and_no_setup_url(self):
        with tempfile.TemporaryDirectory() as directory:
            service = bootstrap.Bootstrap(directory, MANIFEST, 3600)
            service.origin = "http://127.0.0.1:12345"
            form = service.form_html()
            self.assertIn("https://github.com/organizations/ajent-social/settings/apps/new?state=", form)
            self.assertIn(service.callback_path, form)
            self.assertNotIn("setup_url", form)
            self.assertGreater(len(service.form_path), 40)

    def test_redirects_are_rejected_without_following(self):
        with self.assertRaises(bootstrap.BootstrapError):
            bootstrap.NoRedirect().redirect_request(None, None, 302, "redirect", {}, "https://evil.invalid")

    def test_api_http_error_never_exposes_conversion_code(self):
        error = bootstrap.urllib.error.HTTPError("https://api.github.com/app-manifests/" + "a" * 40 + "/conversions", 400, "bad", {}, None)
        opener = mock.Mock()
        opener.open.side_effect = error
        with mock.patch.object(bootstrap.urllib.request, "build_opener", return_value=opener), self.assertRaises(bootstrap.BootstrapError) as raised:
            bootstrap.api_request("POST", "/app-manifests/" + "a" * 40 + "/conversions")
        self.assertNotIn("a" * 40, str(raised.exception))


class CryptoTests(unittest.TestCase):
    @unittest.skipUnless(importlib.util.find_spec("cryptography"), "operator crypto dependency unavailable")
    def test_operator_rsa_loader_and_client_id_jwt(self):
        from cryptography.hazmat.primitives import serialization, hashes
        from cryptography.hazmat.primitives.asymmetric import padding, rsa
        import base64
        key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        pem = key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()).decode()
        loaded, fingerprint = bootstrap.load_private_key(pem)
        self.assertEqual(len(fingerprint), 64)
        jwt = bootstrap.app_jwt(loaded, "Iv1.public-client", now=1000)
        header, payload, signature = jwt.split(".")
        decoded = json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))
        self.assertEqual(decoded, {"iat": 940, "exp": 1540, "iss": "Iv1.public-client"})
        raw_signature = base64.urlsafe_b64decode(signature + "=" * (-len(signature) % 4))
        key.public_key().verify(raw_signature, (header + "." + payload).encode(), padding.PKCS1v15(), hashes.SHA256())


class KeyAndScopeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.key = object()
        cls.pem = "-----BEGIN PRIVATE KEY-----\nfixture-not-a-real-key\n-----END PRIVATE KEY-----"

    def setUp(self):
        loader = mock.patch.object(bootstrap, "load_private_key", return_value=(self.key, "f" * 64))
        jwt = mock.patch.object(bootstrap, "app_jwt", return_value="fixture-jwt")
        loader.start()
        jwt.start()
        self.addCleanup(loader.stop)
        self.addCleanup(jwt.stop)

    def test_app_validation_rejects_wrong_owner_or_permissions(self):
        key, public = bootstrap.validate_app(app_data(self.pem))
        self.assertIs(key, self.key)
        self.assertNotIn("pem", public)
        self.assertNotIn("client_secret", public)
        self.assertEqual(len(public["public_key_sha256"]), 64)
        for field, value in (("owner", {"login": "other", "id": 1}), ("permissions", {"contents": "write"}), ("pem", "")):
            data = app_data(self.pem)
            data[field] = value
            with self.subTest(field=field), self.assertRaises(bootstrap.BootstrapError):
                bootstrap.validate_app(data)

    def test_private_key_only_goes_to_control_secret_stdin(self):
        calls = []
        def run(args, **kwargs):
            calls.append((args, kwargs))
            if args[1] == "api":
                return SimpleNamespace(stdout=json.dumps({"full_name": bootstrap.CONTROL_REPO, "private": True, "allow_forking": False}))
            return SimpleNamespace(stdout="")
        with mock.patch.dict(os.environ, {"GH_DEBUG": "api"}):
            bootstrap.store_control_secret(self.pem, run=run)
        self.assertEqual(calls[1][1]["input"], self.pem)
        self.assertTrue(all(self.pem not in arg for args, _ in calls for arg in args))
        self.assertIn(bootstrap.CONTROL_REPO, calls[1][0])
        self.assertNotIn("GH_DEBUG", calls[1][1]["env"])
        self.assertEqual(calls[1][0][1:4], ["secret", "set", "AJENT_SOURCE_APP_PRIVATE_KEY"])

    def test_public_control_repo_is_rejected_before_secret_set(self):
        calls = []
        def run(args, **kwargs):
            calls.append(args)
            return SimpleNamespace(stdout=json.dumps({"full_name": bootstrap.CONTROL_REPO, "private": False, "allow_forking": False}))
        with self.assertRaises(bootstrap.BootstrapError):
            bootstrap.store_control_secret(self.pem, run=run)
        self.assertEqual(len(calls), 1)

    def test_installation_requires_single_selected_expected_org(self):
        valid = installation()
        self.assertEqual(bootstrap.validate_installation([valid], 123)["id"], 456)
        for collection in ([valid, valid], [dict(valid, repository_selection="all")],
                           [dict(valid, permissions={"contents": "write"})],
                           [dict(valid, account={"id": 1, "login": "other"})]):
            with self.subTest(collection=collection), self.assertRaises(bootstrap.BootstrapError):
                bootstrap.validate_installation(collection, 123)

    def verify(self, *, repos=None, permissions=None, fail_main=False):
        calls = []
        expires = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat()
        def api(method, path, **kwargs):
            calls.append((method, path, kwargs))
            if path.endswith("/access_tokens"):
                return {"token": "ephemeral-test-token", "permissions": permissions or dict(bootstrap.PERMISSIONS), "expires_at": expires}
            if path.startswith("/installation/repositories"):
                return repos or {"total_count": 1, "repositories": [{"id": bootstrap.SOURCE_REPO_ID, "full_name": bootstrap.SOURCE_REPO}]}
            if path.endswith("/commits/main"):
                if fail_main:
                    raise bootstrap.BootstrapError("Read failed")
                return {"sha": "b" * 40}
            if "/actions/runs" in path:
                return {"total_count": 0, "workflow_runs": []}
            if path == "/installation/token":
                return None
            raise AssertionError(path)
        stages = []
        result = None
        try:
            result = bootstrap.verify_source_access(installation(), {"client_id": "Iv1.public"}, self.key, api=api,
                                                     on_stage=lambda stage, **public: stages.append((stage, public)))
        finally:
            self.assertEqual(calls[-1][0:2], ("DELETE", "/installation/token"))
            self.assertEqual(calls[-1][2]["token"], "ephemeral-test-token")
            self.assertEqual(stages[-1][1]["token_revocation"], "REVOKED")
        return result, calls

    def test_exact_source_verification_and_exact_token_revocation(self):
        result, calls = self.verify()
        self.assertTrue(result["contents_verified"])
        self.assertTrue(result["actions_verified"])
        issued = calls[0]
        self.assertEqual(issued[2]["body"], {"repositories": ["ajent-social"], "permissions": {"contents": "read", "actions": "read"}})
        self.assertNotIn("ephemeral-test-token", json.dumps(result))

    def test_scope_or_read_failure_still_revokes_exact_token(self):
        for kwargs in ({"repos": {"total_count": 2, "repositories": []}},
                       {"permissions": {"contents": "write"}}, {"fail_main": True}):
            with self.subTest(kwargs=kwargs), self.assertRaises(bootstrap.BootstrapError):
                self.verify(**kwargs)

    def test_success_receipt_never_contains_key_unused_secrets_code_or_tokens(self):
        stored = []
        expires = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat()
        def api(method, path, **kwargs):
            if path.startswith("/app-manifests/"):
                return app_data(self.pem)
            if path == "/app":
                return app_data(self.pem)
            if path.startswith("/app/installations?"):
                return [installation()]
            if path.endswith("/access_tokens"):
                return {"token": "ephemeral-test-token", "permissions": dict(bootstrap.PERMISSIONS), "expires_at": expires}
            if path.startswith("/installation/repositories"):
                return {"total_count": 1, "repositories": [{"id": bootstrap.SOURCE_REPO_ID, "full_name": bootstrap.SOURCE_REPO}]}
            if path.endswith("/commits/main"):
                return {"sha": "b" * 40}
            if "/actions/runs" in path:
                return {"total_count": 1, "workflow_runs": []}
            return None
        with tempfile.TemporaryDirectory() as directory:
            service = bootstrap.Bootstrap(directory, MANIFEST, 3600, api=api,
                                          store_secret=lambda value, **kwargs: stored.append(value))
            service.convert_and_verify("a" * 40)
            self.assertEqual(service.receipt["stage"], "COMPLETE")
            raw = service.path.read_text()
            for secret in (self.pem, "unused-oauth-secret", "unused-webhook-secret", "ephemeral-test-token", "fixture-jwt", "a" * 40):
                self.assertNotIn(secret, raw)
            self.assertEqual(stored, [self.pem])
            self.assertEqual(service.receipt["verification"]["full_installation_repository_list_proof"], "OWNER_BROWSER_REQUIRED")

    def test_deadline_blocks_new_calls_but_permits_exact_token_revocation(self):
        calls = []
        clock = [0]
        with tempfile.TemporaryDirectory() as directory:
            service = bootstrap.Bootstrap(directory, MANIFEST, 60, monotonic=lambda: clock[0],
                                          api=lambda method, path, **kwargs: calls.append((method, path)))
            clock[0] = 61
            with self.assertRaises(bootstrap.BootstrapError):
                service.api("GET", "/app")
            service.api("DELETE", "/installation/token", token="test")
            self.assertEqual(calls, [("DELETE", "/installation/token")])


if __name__ == "__main__":
    unittest.main()
