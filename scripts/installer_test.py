#!/usr/bin/env python3
"""Regression tests for the signed Unix and PowerShell release installers.

The suite builds local release fixtures and serves them over loopback. It does
not contact the release service or execute a downloaded release binary.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import http.server
import io
import os
from pathlib import Path
import platform
import shutil
import shlex
import subprocess
import sys
import tarfile
import tempfile
import threading
import unittest
import zipfile


VERSION = "9.8.7"
INSTALLER_DIR = Path(
    os.environ.get("AJENT_INSTALLER_TEST_INSTALLER_DIR")
    or Path(__file__).resolve().parents[1] / "internal/web/onboarding"
)
POWERSHELL = os.environ.get("AJENT_INSTALLER_TEST_PWSH") or shutil.which("pwsh")
if os.environ.get("AJENT_INSTALLER_TEST_REQUIRE_PWSH") == "1" and not POWERSHELL:
    raise RuntimeError("AJENT_INSTALLER_TEST_REQUIRE_PWSH=1 but no pwsh executable was found")


def command(args: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, text=True, capture_output=True, check=True, **kwargs)


def ed25519_keys(root: Path) -> tuple[Path, Path, Path]:
    openssl = shutil.which("openssl")
    if not openssl:
        raise RuntimeError("OpenSSL is required to create signed installer fixtures")
    private_key = root / "fixture-private.pem"
    public_key = root / "fixture-public.pem"
    wrong_public_key = root / "wrong-fixture-public.pem"
    wrong_private_key = root / "wrong-fixture-private.pem"
    command([openssl, "genpkey", "-algorithm", "Ed25519", "-out", str(private_key)])
    command([openssl, "pkey", "-in", str(private_key), "-pubout", "-out", str(public_key)])
    command([openssl, "genpkey", "-algorithm", "Ed25519", "-out", str(wrong_private_key)])
    command([openssl, "pkey", "-in", str(wrong_private_key), "-pubout", "-out", str(wrong_public_key)])
    return private_key, public_key, wrong_public_key


class ReleaseServer(http.server.ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self) -> None:
        self.files: dict[str, bytes] = {}
        self.requests: list[str] = []
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                owner.requests.append(self.path)
                body = owner.files.get(self.path)
                if body is None:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, _format: str, *_args: object) -> None:
                pass

        super().__init__(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.serve_forever, daemon=True)
        self.thread.start()

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.server_port}/releases"

    def close(self) -> None:
        self.shutdown()
        self.server_close()
        self.thread.join(timeout=5)


class SignedInstallerTests(unittest.TestCase):
    testInstallerDir = INSTALLER_DIR

    @classmethod
    def setUpClass(cls) -> None:
        temp_parent = Path(os.environ.get("TMPDIR", tempfile.gettempdir()))
        temp_parent.mkdir(parents=True, exist_ok=True)
        cls.root = Path(tempfile.mkdtemp(prefix="ajent-installer-tests-", dir=temp_parent))
        cls.private_key, cls.public_key, cls.wrong_public_key = ed25519_keys(cls.root)
        cls.openssl = shutil.which("openssl")
        cls.server = ReleaseServer()
        cls.system = platform.system()
        machine = platform.machine().lower()
        cls.arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(machine)
        cls.unix_name = {"Darwin": "darwin", "Linux": "linux"}.get(cls.system)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.close()
        shutil.rmtree(cls.root)

    def setUp(self) -> None:
        self.test_root = Path(tempfile.mkdtemp(prefix="case-", dir=self.root))
        self.marker = self.test_root / "release-executed"
        self.server.requests.clear()

    def tearDown(self) -> None:
        shutil.rmtree(self.test_root)

    @staticmethod
    def shell_payload(marker: Path) -> bytes:
        # If an installer ever executes the downloaded file before verification,
        # this benign fixture leaves a clear marker.
        return f"#!/bin/sh\nprintf executed > {shlex.quote(str(marker))}\n".encode()

    def make_archive(self, kind: str) -> tuple[str, bytes]:
        if kind == "unix":
            self.assertIn(self.system, ("Darwin", "Linux"), "Unix installer tests need macOS or Linux")
            self.assertIsNotNone(self.arch, f"unsupported test architecture: {platform.machine()}")
            name = f"ajent_{VERSION}_{self.unix_name}_{self.arch}.tar.gz"
            output = io.BytesIO()
            with tarfile.open(fileobj=output, mode="w:gz") as archive:
                payload = self.shell_payload(self.marker)
                info = tarfile.TarInfo("ajent")
                info.size = len(payload)
                info.mode = 0o755
                archive.addfile(info, io.BytesIO(payload))
            return name, output.getvalue()

        name = f"ajent_{VERSION}_windows_amd64.zip"
        output = io.BytesIO()
        with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            archive.writestr("ajent.exe", self.shell_payload(self.marker))
        return name, output.getvalue()

    def make_release(
        self,
        kind: str,
        *,
        checksum: str = "correct",
        signature: str = "valid",
    ) -> str:
        name, archive = self.make_archive(kind)
        digest = hashlib.sha256(archive).hexdigest()
        if checksum == "wrong":
            digest = "0" * 64 if digest != "0" * 64 else "1" * 64
        checksums = f"{digest}  {name}\n".encode()
        checksums_path = self.test_root / "checksums.txt"
        signature_path = self.test_root / "checksums.sig"
        checksums_path.write_bytes(checksums)
        command(
            [
                str(self.openssl), "pkeyutl", "-sign", "-rawin", "-inkey", str(self.private_key),
                "-in", str(checksums_path), "-out", str(signature_path),
            ]
        )
        signature_bytes = signature_path.read_bytes()
        if signature == "tampered":
            signature_bytes = bytes([signature_bytes[0] ^ 1]) + signature_bytes[1:]
        sig_text = base64.b64encode(signature_bytes) + b"\n"
        prefix = f"/releases/v{VERSION}/"
        self.server.files = {
            prefix + name: archive,
            prefix + "checksums.txt": checksums,
            prefix + "checksums.txt.sig": sig_text,
        }
        return name

    def installer_dir(self) -> Path:
        return Path(self.testInstallerDir)

    def installer_env(self, *, public_key: Path | None, platform_kind: str) -> dict[str, str]:
        install_dir = self.test_root / "installed"
        config_dir = self.test_root / "config"
        temp_dir = self.test_root / "tmp"
        temp_dir.mkdir(exist_ok=True)
        env = os.environ.copy()
        env.update(
            {
                "AJENT_VERSION": VERSION,
                "AJENT_DOWNLOAD_BASE": self.server.base_url,
                "AJENT_INSTALL_DIR": str(install_dir),
                "AJENT_CONFIG_DIR": str(config_dir),
                "AJENT_NO_SETUP": "1",
                "AJENT_TEST_MARKER": str(self.marker),
                "TMPDIR": str(temp_dir),
                "TEMP": str(temp_dir),
                "TMP": str(temp_dir),
                "NO_PROXY": "127.0.0.1,localhost",
                "no_proxy": "127.0.0.1,localhost",
            }
        )
        if public_key is None:
            env.pop("AJENT_RELEASE_PUBKEY_FILE", None)
        else:
            env["AJENT_RELEASE_PUBKEY_FILE"] = str(public_key)
        if platform_kind == "powershell":
            env["PROCESSOR_ARCHITECTURE"] = "AMD64"
            env["LOCALAPPDATA"] = str(self.test_root / "local-app-data")
        return env

    def run_unix_installer(self, env: dict[str, str], *arguments: str) -> subprocess.CompletedProcess[str]:
        installer = self.installer_dir() / "install.sh"
        return subprocess.run(
            ["/bin/sh", str(installer), *arguments], text=True, capture_output=True, env=env, timeout=45
        )

    def run_powershell_installer(
        self, env: dict[str, str], *arguments: str
    ) -> subprocess.CompletedProcess[str]:
        if not POWERSHELL:
            raise RuntimeError("pwsh is required; install PowerShell or set AJENT_INSTALLER_TEST_PWSH")
        installer = self.installer_dir() / "install.ps1"
        return subprocess.run(
            [POWERSHELL, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(installer), *arguments],
            text=True,
            capture_output=True,
            env=env,
            timeout=60,
        )

    def assert_rejected(self, result: subprocess.CompletedProcess[str], installed_name: str) -> None:
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.test_root / "installed" / installed_name).exists())
        self.assertFalse((self.test_root / "config" / "install.json").exists())
        self.assertFalse(self.marker.exists(), "the downloaded fixture must never be executed")

    def without_openssl_path(self) -> str:
        """Provide the Unix installer tools while withholding OpenSSL."""
        path = self.test_root / "path-without-openssl"
        path.mkdir(exist_ok=True)
        required = (
            "curl", "uname", "tar", "gzip", "mktemp", "awk", "grep", "cp", "mkdir",
            "chmod", "mv", "date", "rm", "cat", "shasum", "sha256sum", "sysctl",
        )
        source_path = os.environ.get("PATH", "")
        for name in required:
            source = shutil.which(name, path=source_path)
            if source:
                link = path / name
                if not link.exists():
                    link.symlink_to(source)
        return str(path)

    def assert_installed(self, name: str) -> None:
        binary = self.test_root / "installed" / name
        self.assertTrue(binary.is_file())
        self.assertTrue((self.test_root / "config" / "install.json").is_file())
        self.assertFalse(self.marker.exists(), "installer must not execute the release archive")

    @unittest.skipUnless(platform.system() in ("Darwin", "Linux"), "Unix installer supports macOS and Linux")
    def test_unix_accepts_a_correctly_signed_release(self) -> None:
        name = self.make_release("unix")
        result = self.run_unix_installer(self.installer_env(public_key=self.public_key, platform_kind="unix"))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_installed("ajent")

    @unittest.skipUnless(platform.system() in ("Darwin", "Linux"), "Unix installer supports macOS and Linux")
    def test_unix_fails_closed_for_signature_key_checksum_and_verifier_errors(self) -> None:
        cases = (
            ("forged signature", {"signature": "tampered"}, self.public_key, False),
            ("wrong signing key", {}, self.wrong_public_key, False),
            ("missing key", {}, None, False),
            ("signed but wrong checksum", {"checksum": "wrong"}, self.public_key, False),
            ("missing OpenSSL verifier", {}, self.public_key, True),
        )
        for label, release_args, key, hide_openssl in cases:
            with self.subTest(case=label):
                original_root, original_marker = self.test_root, self.marker
                self.test_root = Path(tempfile.mkdtemp(prefix="case-", dir=self.root))
                self.marker = self.test_root / "release-executed"
                try:
                    self.make_release("unix", **release_args)
                    env = self.installer_env(public_key=key, platform_kind="unix")
                    if hide_openssl:
                        env["PATH"] = self.without_openssl_path()
                    result = self.run_unix_installer(env)
                    self.assert_rejected(result, "ajent")
                finally:
                    shutil.rmtree(self.test_root)
                    self.test_root, self.marker = original_root, original_marker

    @unittest.skipUnless(platform.system() in ("Darwin", "Linux"), "Unix installer supports macOS and Linux")
    def test_unix_rejects_unused_options_before_downloading(self) -> None:
        self.make_release("unix")
        env = self.installer_env(public_key=self.public_key, platform_kind="unix")
        result = self.run_unix_installer(env, "--unknown-installer-option")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.server.requests, [])
        self.assert_rejected(result, "ajent")

    @unittest.skipUnless(POWERSHELL, "pwsh is required for PowerShell installer regression tests")
    def test_powershell_accepts_a_correctly_signed_release(self) -> None:
        name = self.make_release("windows")
        result = self.run_powershell_installer(
            self.installer_env(public_key=self.public_key, platform_kind="powershell")
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_installed("ajent.exe")

    @unittest.skipUnless(POWERSHELL, "pwsh is required for PowerShell installer regression tests")
    def test_powershell_fails_closed_for_signature_key_checksum_and_verifier_errors(self) -> None:
        cases = (
            ("forged signature", {"signature": "tampered"}, self.public_key, False),
            ("wrong signing key", {}, self.wrong_public_key, False),
            ("missing key", {}, None, False),
            ("signed but wrong checksum", {"checksum": "wrong"}, self.public_key, False),
            ("missing OpenSSL verifier", {}, self.public_key, True),
        )
        for label, release_args, key, hide_openssl in cases:
            with self.subTest(case=label):
                original_root, original_marker = self.test_root, self.marker
                self.test_root = Path(tempfile.mkdtemp(prefix="case-", dir=self.root))
                self.marker = self.test_root / "release-executed"
                try:
                    self.make_release("windows", **release_args)
                    env = self.installer_env(public_key=key, platform_kind="powershell")
                    if hide_openssl:
                        env["PATH"] = str(self.test_root / "path-without-openssl")
                        Path(env["PATH"]).mkdir()
                    result = self.run_powershell_installer(env)
                    self.assert_rejected(result, "ajent.exe")
                finally:
                    shutil.rmtree(self.test_root)
                    self.test_root, self.marker = original_root, original_marker

    @unittest.skipUnless(POWERSHELL, "pwsh is required for PowerShell installer regression tests")
    def test_powershell_rejects_unused_options_before_downloading(self) -> None:
        self.make_release("windows")
        env = self.installer_env(public_key=self.public_key, platform_kind="powershell")
        result = self.run_powershell_installer(env, "-UnknownInstallerOption")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.server.requests, [])
        self.assert_rejected(result, "ajent.exe")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--installer-dir", type=Path, default=INSTALLER_DIR)
    args, unittest_args = parser.parse_known_args()
    SignedInstallerTests.testInstallerDir = args.installer_dir.resolve()
    for script_name in ("install.sh", "install.ps1"):
        if not (SignedInstallerTests.testInstallerDir / script_name).is_file():
            parser.error(f"{script_name} not found in {SignedInstallerTests.testInstallerDir}")
    sys.argv = [sys.argv[0], *unittest_args]
    unittest.main()


if __name__ == "__main__":
    main()
