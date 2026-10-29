#!/usr/bin/env python3
"""Bounded owner-assisted GitHub App manifest bootstrap; credentials stay in RAM.

The only standing key destination is the Actions secret in the private release
control repository. Receipts contain public metadata and stage outcomes only.
"""
import argparse
import base64
import datetime
import hashlib
import hmac
import html
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import re
import resource
import secrets
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ORG = "ajent-social"
ORG_ID = 325803062
SOURCE_REPO = "ajent-social/ajent-social"
SOURCE_REPO_ID = 1359554173
CONTROL_REPO = "ajent-social/production-release"
PERMISSIONS = {"contents": "read", "actions": "read", "metadata": "read"}
APP_NAME = "Ajent Production Source Read"


class BootstrapError(Exception):
    """A safe, fixed public reason; never wrap provider response text."""


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise BootstrapError("GitHub API redirect rejected")


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def b64url(value):
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def app_jwt(key, client_id, now=None):
    from cryptography.hazmat.primitives import hashes
    from cryptography.hazmat.primitives.asymmetric import padding
    now = int(time.time() if now is None else now)
    header = b64url(json.dumps({"alg": "RS256", "typ": "JWT"}, separators=(",", ":")).encode())
    payload = b64url(json.dumps({"iat": now - 60, "exp": now + 540, "iss": client_id}, separators=(",", ":")).encode())
    unsigned = (header + "." + payload).encode("ascii")
    signature = key.sign(unsigned, padding.PKCS1v15(), hashes.SHA256())
    return unsigned.decode("ascii") + "." + b64url(signature)


def api_request(method, path, *, token=None, body=None, timeout=20):
    if not path.startswith("/") or path.startswith("//"):
        raise BootstrapError("Invalid GitHub API path")
    headers = {"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28",
               "User-Agent": "Ajent-Source-App-Bootstrap"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request("https://api.github.com" + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.build_opener(NoRedirect).open(request, timeout=timeout) as response:
            if 300 <= response.status < 400:
                raise BootstrapError("GitHub API redirect rejected")
            raw = response.read(2 * 1024 * 1024 + 1)
            if len(raw) > 2 * 1024 * 1024:
                raise BootstrapError("GitHub API response too large")
            return json.loads(raw) if raw else None
    except BootstrapError:
        raise
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, ValueError):
        # HTTPError includes the conversion-code URL and may carry provider
        # details. Keep both the exception and response body out of logs.
        raise BootstrapError("GitHub API request failed") from None


def validate_manifest(manifest):
    expected = {"name", "url", "description", "public", "hook_attributes", "default_permissions",
                "default_events", "request_oauth_on_install", "setup_on_update"}
    if (set(manifest) != expected or manifest.get("name") != APP_NAME or manifest.get("url") != "https://ajent.social"
            or manifest.get("public") is not False or manifest.get("default_permissions") != PERMISSIONS
            or manifest.get("default_events") != [] or manifest.get("request_oauth_on_install") is not False
            or manifest.get("setup_on_update") is not False
            or manifest.get("hook_attributes") != {"url": "https://ajent.social", "active": False}):
        raise BootstrapError("Manifest differs from approved read-only App configuration")


def validate_app(data):
    if not isinstance(data, dict):
        raise BootstrapError("App conversion response missing")
    owner = data.get("owner", {})
    if owner.get("login") != ORG or owner.get("id") != ORG_ID:
        raise BootstrapError("App owner differs from approved organization")
    if data.get("permissions") != PERMISSIONS:
        raise BootstrapError("App permissions differ from approved read-only scope")
    app_id, client_id, slug = data.get("id"), data.get("client_id"), data.get("slug")
    if (not isinstance(app_id, int) or isinstance(app_id, bool) or app_id <= 0
            or not isinstance(client_id, str) or not re.fullmatch(r"[A-Za-z0-9_.-]{1,100}", client_id)
            or not isinstance(slug, str) or not re.fullmatch(r"[a-z0-9-]{1,100}", slug)
            or data.get("html_url") != "https://github.com/apps/" + slug):
        raise BootstrapError("App public identity is invalid")
    pem = data.get("pem")
    if not isinstance(pem, str) or not pem.strip():
        raise BootstrapError("App conversion did not return a private key")
    key, fingerprint = load_private_key(pem)
    return key, {"id": app_id, "client_id": client_id, "slug": slug, "html_url": data["html_url"],
                 "permissions": dict(PERMISSIONS), "public_key_sha256": fingerprint,
                 "installation_url": data["html_url"] + "/installations/new"}


def load_private_key(pem):
    # Only the operator's conversion path requires cryptography. Importing the
    # helper and running its mocked protocol/scope tests needs the stdlib only.
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import rsa
    try:
        key = serialization.load_pem_private_key(pem.encode("ascii"), password=None)
    except (ValueError, TypeError, UnicodeError):
        raise BootstrapError("App private key is invalid") from None
    if not isinstance(key, rsa.RSAPrivateKey) or key.key_size < 2048:
        raise BootstrapError("App key must be RSA with at least 2048 bits")
    public = key.public_key().public_bytes(serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)
    return key, hashlib.sha256(public).hexdigest()


def store_control_secret(pem, *, run=subprocess.run, deadline=None, monotonic=time.monotonic):
    # Disable CLI tracing/debugging before invoking GitHub. Key input uses a
    # pipe, never argv, disk, Pulumi configuration or provider state.
    environment = {k: v for k, v in os.environ.items() if k not in ("GH_DEBUG", "GH_TRACE", "GIT_TRACE", "GIT_CURL_VERBOSE")}
    environment.update(GH_HOST="github.com", GH_PROMPT_DISABLED="1")
    def remaining():
        budget = 30 if deadline is None else deadline - monotonic()
        if budget <= 0:
            raise BootstrapError("Bootstrap deadline elapsed before secret storage")
        return min(30, budget)
    try:
        metadata = run(["gh", "api", "repos/" + CONTROL_REPO], capture_output=True, text=True,
                       check=True, timeout=remaining(), env=environment)
        repo = json.loads(metadata.stdout)
        if (repo.get("full_name") != CONTROL_REPO or repo.get("private") is not True
                or repo.get("allow_forking") is not False):
            raise BootstrapError("Control repository must be private with forking disabled")
        run(["gh", "secret", "set", "AJENT_SOURCE_APP_PRIVATE_KEY", "--repo", CONTROL_REPO],
            input=pem, capture_output=True, text=True, check=True, timeout=remaining(), env=environment)
    except BootstrapError:
        raise
    except (subprocess.SubprocessError, ValueError):
        raise BootstrapError("Control-repository key secret could not be stored") from None


def validate_installation(installations, app_id):
    if not isinstance(installations, list) or len(installations) > 1:
        raise BootstrapError("App must have at most one installation")
    if not installations:
        return None
    installation = installations[0]
    account = installation.get("account", {})
    if (installation.get("app_id") != app_id or account.get("login") != ORG or account.get("id") != ORG_ID
            or installation.get("repository_selection") != "selected" or installation.get("permissions") != PERMISSIONS
            or installation.get("suspended_at") is not None or not isinstance(installation.get("id"), int)
            or isinstance(installation.get("id"), bool) or installation["id"] <= 0):
        raise BootstrapError("Installation differs from approved selected-repository scope")
    return installation


def validate_token_response(response):
    if not isinstance(response, dict) or response.get("permissions") != PERMISSIONS:
        raise BootstrapError("Installation token permissions differ from approved scope")
    expires_at = response.get("expires_at")
    try:
        expiry = datetime.datetime.fromisoformat(expires_at.replace("Z", "+00:00"))
        lifetime = (expiry - datetime.datetime.now(datetime.timezone.utc)).total_seconds()
    except (TypeError, AttributeError, ValueError):
        raise BootstrapError("Installation token expiry is invalid") from None
    if not 0 < lifetime <= 3605:
        raise BootstrapError("Installation token exceeds one-hour lifetime")


def verify_source_access(installation, app, key, *, api=api_request, on_stage=None):
    token = None
    revocation = "NOT_ISSUED"
    outcome = None
    primary_error = None
    try:
        response = api("POST", f"/app/installations/{installation['id']}/access_tokens",
                       token=app_jwt(key, app["client_id"]),
                       body={"repositories": ["ajent-social"], "permissions": {"contents": "read", "actions": "read"}})
        if isinstance(response, dict):
            token = response.get("token")
        if not isinstance(token, str) or not token:
            raise BootstrapError("Installation access token missing")
        validate_token_response(response)
        repositories = api("GET", "/installation/repositories?per_page=100", token=token)
        repos = repositories.get("repositories", []) if isinstance(repositories, dict) else []
        if (not isinstance(repositories, dict) or repositories.get("total_count") != 1 or len(repos) != 1
                or repos[0].get("id") != SOURCE_REPO_ID or repos[0].get("full_name") != SOURCE_REPO):
            raise BootstrapError("Installation token does not expose exactly the approved source repository")
        commit = api("GET", "/repos/" + SOURCE_REPO + "/commits/main", token=token)
        sha = commit.get("sha", "") if isinstance(commit, dict) else ""
        if not re.fullmatch(r"[a-f0-9]{40}", sha):
            raise BootstrapError("Source main commit could not be read")
        runs = api("GET", "/repos/" + SOURCE_REPO + "/actions/runs?per_page=1", token=token)
        if not isinstance(runs, dict) or not isinstance(runs.get("workflow_runs"), list) or not isinstance(runs.get("total_count"), int):
            raise BootstrapError("Source Actions results could not be read")
        outcome = {"installation_id": installation["id"], "source_repository_id": SOURCE_REPO_ID,
                   "source_repository": SOURCE_REPO, "observed_main": sha, "contents_verified": True,
                   "actions_verified": True, "token_expires_at": response["expires_at"],
                   "token_repository_scope_verified": True, "installation_selection_verified": "selected",
                   "full_installation_repository_list_proof": "OWNER_BROWSER_REQUIRED"}
    except BaseException as error:
        primary_error = error
    finally:
        if token:
            try:
                api("DELETE", "/installation/token", token=token)
                revocation = "REVOKED"
            except BaseException:
                revocation = "NEEDS_RECONCILIATION"
                if primary_error is None:
                    primary_error = BootstrapError("Verification token revocation requires reconciliation")
        if on_stage:
            on_stage("token_verification", token_revocation=revocation)
        token = None
    if primary_error:
        if isinstance(primary_error, BootstrapError):
            raise primary_error
        raise BootstrapError("Source installation verification failed") from None
    return outcome


class Bootstrap:
    def __init__(self, receipt_dir, manifest, timeout, *, api=api_request, store_secret=store_control_secret,
                 monotonic=time.monotonic, sleep=time.sleep):
        validate_manifest(manifest)
        self.directory = Path(receipt_dir)
        self.directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        if self.directory.is_symlink() or self.directory.stat().st_mode & 0o777 != 0o700:
            raise BootstrapError("Receipt directory must be a private non-symlink directory")
        self.path = self.directory / "receipt.json"
        fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        self.receipt = {"started_at": utc_now(), "stage": "AWAITING_REGISTRATION", "app": None,
                        "full_installation_repository_list_proof": "OWNER_BROWSER_REQUIRED"}
        with os.fdopen(fd, "w") as stream:
            json.dump(self.receipt, stream, indent=2)
        self.manifest = dict(manifest)
        self.state = secrets.token_urlsafe(32)
        self.form_path = "/register/" + secrets.token_urlsafe(32)
        self.callback_path = "/callback/" + secrets.token_urlsafe(32)
        self._api, self.store_secret = api, store_secret
        self.monotonic, self.sleep = monotonic, sleep
        self.deadline = monotonic() + timeout
        self.lock = threading.RLock()
        self.consumed = False
        self.worker = None
        self.done = threading.Event()
        self.cancelled = threading.Event()
        self.origin = None

    def api(self, method, path, **kwargs):
        remaining = self.deadline - self.monotonic()
        # Revoking this helper's exact verification token remains permitted
        # during bounded cleanup after the registration deadline.
        if method == "DELETE" and path == "/installation/token":
            budget = 20
        elif remaining <= 0 or self.cancelled.is_set():
            raise BootstrapError("Bootstrap deadline elapsed or owner stopped helper")
        else:
            budget = min(20, remaining)
        return self._api(method, path, timeout=budget, **kwargs)

    def stage(self, stage, **public):
        with self.lock:
            self.receipt.update(public)
            self.receipt["stage"] = stage
            self.receipt["updated_at"] = utc_now()
            with tempfile.NamedTemporaryFile(mode="w", dir=self.directory, delete=False) as stream:
                json.dump(self.receipt, stream, indent=2)
                stream.write("\n")
                temporary = stream.name
            os.chmod(temporary, 0o600)
            os.replace(temporary, self.path)

    def accept_callback(self, query):
        with self.lock:
            if self.monotonic() >= self.deadline:
                return 410
            if set(query) != {"state", "code"} or any(len(values) != 1 for values in query.values()):
                return 400
            if not re.fullmatch(r"[A-Za-z0-9_-]{43}", query["state"][0]) or not hmac.compare_digest(query["state"][0], self.state):
                return 403
            if not re.fullmatch(r"[a-f0-9]{40}", query["code"][0]):
                return 400
            if self.consumed:
                return 409
            self.consumed = True
            self.stage("CONVERSION_ACCEPTED")
            return 303

    def convert_and_verify(self, code):
        key = pem = None
        try:
            if self.monotonic() >= self.deadline:
                raise BootstrapError("Bootstrap deadline elapsed")
            data = self.api("POST", f"/app-manifests/{code}/conversions")
            code = None
            key, app = validate_app(data)
            pem = data.pop("pem")
            # Never persist or use the unused OAuth/webhook credentials.
            data.clear()
            self.stage("APP_CREATED", app=app)
            # Verify the App JWT authenticates the same public identity before
            # writing its private key to the only approved standing store.
            authenticated = self.api("GET", "/app", token=app_jwt(key, app["client_id"]))
            if (authenticated.get("id") != app["id"] or authenticated.get("client_id") != app["client_id"]
                    or authenticated.get("permissions") != PERMISSIONS
                    or authenticated.get("owner", {}).get("id") != ORG_ID):
                raise BootstrapError("Authenticated App identity or permissions differ")
            self.store_secret(pem, deadline=self.deadline, monotonic=self.monotonic)
            pem = None
            self.stage("AWAITING_SELECTED_INSTALLATION", key_secret_stored=True)
            while self.monotonic() < self.deadline:
                installations = self.api("GET", "/app/installations?per_page=100", token=app_jwt(key, app["client_id"]))
                installation = validate_installation(installations, app["id"])
                if installation:
                    self.stage("VERIFYING_SELECTED_INSTALLATION", installation_id=installation["id"])
                    outcome = verify_source_access(installation, app, key, api=self.api, on_stage=self.stage)
                    self.stage("COMPLETE", verification=outcome)
                    return
                wait = min(10, max(0, self.deadline - self.monotonic()))
                if self.sleep is time.sleep:
                    self.cancelled.wait(wait)
                else:
                    self.sleep(wait)
            raise BootstrapError("Selected installation was not verified before the one-hour deadline")
        except BootstrapError as error:
            self.stage("NEEDS_RECONCILIATION", reason=str(error))
        except BaseException:
            self.stage("NEEDS_RECONCILIATION", reason="Bootstrap failed; inspect public App identity before retrying")
        finally:
            key = pem = code = None
            self.done.set()

    def form_html(self):
        manifest = dict(self.manifest, redirect_url=self.origin + self.callback_path)
        encoded = html.escape(json.dumps(manifest, separators=(",", ":")), quote=True)
        action = "https://github.com/organizations/ajent-social/settings/apps/new?state=" + urllib.parse.quote(self.state)
        return ("<!doctype html><title>Register Ajent source App</title><h1>Register read-only Ajent source App</h1>"
                "<p>The App requests only Contents, Actions and mandatory Metadata read access.</p>"
                f'<form action="{html.escape(action, quote=True)}" method="post">'
                f'<input type="hidden" name="manifest" value="{encoded}">'
                '<button type="submit">Register approved GitHub App</button></form>')

    def status_html(self):
        with self.lock:
            stage = self.receipt["stage"]
            app = self.receipt.get("app")
            link = (f'<p><a href="{html.escape(app["installation_url"], quote=True)}">Install on only ajent-social/ajent-social</a></p>'
                    if app and stage == "AWAITING_SELECTED_INSTALLATION" else "")
            refresh = '<meta http-equiv="refresh" content="3;url=/waiting">' if not self.done.is_set() else ""
            return f'<!doctype html><title>Ajent source App bootstrap</title>{refresh}<h1>{html.escape(stage)}</h1>{link}<p>Credentials are never displayed. Public evidence is in the private receipt directory.</p>'


def handler_for(bootstrap):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, _format, *args):
            pass

        def respond(self, status, body="", location=None):
            self.send_response(status)
            self.send_header("Cache-Control", "no-store")
            self.send_header("Referrer-Policy", "no-referrer")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("Content-Security-Policy", "default-src 'none'; form-action https://github.com; frame-ancestors 'none'; base-uri 'none'")
            if location:
                self.send_header("Location", location)
            raw = body.encode("utf-8")
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            if raw:
                self.wfile.write(raw)

        def do_GET(self):
            if self.headers.get("Host") != urllib.parse.urlsplit(bootstrap.origin).netloc:
                self.respond(400)
                return
            parsed = urllib.parse.urlsplit(self.path)
            if parsed.path == bootstrap.form_path and not parsed.query:
                if bootstrap.consumed or bootstrap.done.is_set():
                    self.respond(409)
                else:
                    self.respond(200, bootstrap.form_html())
            elif parsed.path == bootstrap.callback_path:
                query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
                status = bootstrap.accept_callback(query)
                if status != 303:
                    self.respond(status)
                    return
                # The browser immediately leaves the code-bearing URL before
                # any network exchange or tool-visible waiting state.
                self.respond(303, location="/waiting")
                bootstrap.worker = threading.Thread(target=bootstrap.convert_and_verify,
                                                    args=(query["code"][0],), daemon=True)
                bootstrap.worker.start()
            elif parsed.path == "/waiting" and not parsed.query:
                self.respond(200, bootstrap.status_html())
            else:
                self.respond(404)

        def do_POST(self):
            self.respond(405)
    return Handler


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--receipt-dir", required=True, type=Path)
    parser.add_argument("--port", type=int, default=0)
    parser.add_argument("--timeout-seconds", type=int, default=3600)
    return parser.parse_args(argv)


def main():
    args = parse_args()
    if not 0 <= args.port <= 65535 or not 30 <= args.timeout_seconds <= 3600:
        raise SystemExit("Port must be 0..65535 and timeout 30..3600 seconds")
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    manifest_path = Path(__file__).resolve().parents[1] / "runners/release/source-app-manifest.json"
    bootstrap = Bootstrap(args.receipt_dir, json.loads(manifest_path.read_text()), args.timeout_seconds)
    server = ThreadingHTTPServer(("127.0.0.1", args.port), handler_for(bootstrap))
    server.timeout = 1
    bootstrap.origin = "http://127.0.0.1:" + str(server.server_address[1])
    print(json.dumps({"registration_url": bootstrap.origin + bootstrap.form_path,
                      "receipt": str(bootstrap.path)}), flush=True)
    try:
        while not bootstrap.done.is_set() and bootstrap.monotonic() < bootstrap.deadline:
            server.handle_request()
        if not bootstrap.done.is_set():
            bootstrap.stage("NEEDS_RECONCILIATION", reason="Bootstrap deadline elapsed")
            if bootstrap.worker:
                bootstrap.worker.join(timeout=45)
        # Briefly keep the clean completion page available after worker exit.
        grace = time.monotonic() + 15
        while time.monotonic() < grace:
            server.handle_request()
    except KeyboardInterrupt:
        bootstrap.cancelled.set()
        bootstrap.stage("NEEDS_RECONCILIATION", reason="Owner stopped bootstrap; reconcile App before retrying")
        if bootstrap.worker:
            bootstrap.worker.join(timeout=45)
    finally:
        server.server_close()
    print(json.dumps({"stage": bootstrap.receipt["stage"], "receipt": str(bootstrap.path)}), flush=True)
    return 0 if bootstrap.receipt["stage"] == "COMPLETE" else 1


if __name__ == "__main__":
    raise SystemExit(main())
