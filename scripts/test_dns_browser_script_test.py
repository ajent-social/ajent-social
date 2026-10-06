import os
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("test-dns-browser.sh")


class DNSBrowserScriptTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ajent-dns-browser-script-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.browser = self.bin / "chromium"
        self.browser.write_text("#!/usr/bin/env bash\nexit 0\n")
        self.browser.chmod(0o755)
        self.log = self.root / "calls.log"
        self._command("createdb", r'''
printf "createdb %s\\n" "$*" >> "$MOCK_LOG"
python3 - "$PGPASSFILE" "$MOCK_PGPASS_OBSERVED" "$MOCK_EXPECT_PASSWORD" <<'PY'
import os
import stat
import sys
from pathlib import Path

path = Path(sys.argv[1])
password = sys.argv[3].replace("\\", "\\\\").replace(":", "\\:")
expected = {
    f"127.0.0.1:55439:ajent_browser:postgres:{password}",
    f"127.0.0.1:55439:ajent_dns_browser_ci_42_1:postgres:{password}",
}
content = set(path.read_text().splitlines())
mode = stat.S_IMODE(path.stat().st_mode)
dir_mode = stat.S_IMODE(path.parent.stat().st_mode)
clean_env = not os.getenv("AJENT_DNS_BROWSER_ADMIN_DSN") and not os.getenv("PGPASSWORD")
Path(sys.argv[2]).write_text(f"{path}\n{mode:o}\n{dir_mode:o}\n{int(expected <= content)}\n{int(clean_env)}\n")
PY
''')
        self._command("dropdb", 'printf "dropdb %s\\n" "$*" >> "$MOCK_LOG"\n')
        self._command("agent-browser", 'printf "agent-browser %s\\n" "$*" >> "$MOCK_LOG"\n')
        self._command("go", '''
printf "go %s\\nDSN %s\\n" "$*" "$AJENT_TEST_DSN" >> "$MOCK_LOG"
if [[ -z "${AJENT_DNS_BROWSER_ADMIN_DSN+x}" && -z "${PGPASSWORD+x}" ]]; then
  printf 'secret-environment=clean\\n' >> "$MOCK_LOG"
else
  printf 'secret-environment=leaked\\n' >> "$MOCK_LOG"
fi
case "${MOCK_GO_MODE:-pass}" in
  pass)
    printf '%s\\n' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof"}' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof/owner"}' \
      '{"Action":"pass","Test":"TestBrowserMatchingDNSProof/owner"}' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof/original"}' \
      '{"Action":"pass","Test":"TestBrowserMatchingDNSProof/original"}' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof/human"}' \
      '{"Action":"pass","Test":"TestBrowserMatchingDNSProof/human"}' \
      '{"Action":"pass","Test":"TestBrowserMatchingDNSProof"}'
    ;;
  empty) printf '%s\\n' '{"Action":"pass","Package":"ajent/internal/web"}' ;;
  skip)
    printf '%s\\n' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof"}' \
      '{"Action":"skip","Test":"TestBrowserMatchingDNSProof"}' \
      '{"Action":"pass","Test":"TestBrowserMatchingDNSProof"}'
    ;;
  fail)
    printf '%s\\n' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof"}' \
      '{"Action":"fail","Test":"TestBrowserMatchingDNSProof"}'
    exit 1
    ;;
  failed-event)
    printf '%s\\n' \
      '{"Action":"run","Test":"TestBrowserMatchingDNSProof"}' \
      '{"Action":"fail","Test":"TestBrowserMatchingDNSProof"}' \
      '{"Action":"pass","Package":"ajent/internal/web"}'
    ;;
esac
''')
        self._command("npm", '''
while (($#)); do
  if [[ "$1" == -- ]]; then shift; break; fi
  if [[ "$1" == --package=agent-browser@* ]]; then
    printf "package %s\\n" "$1" >> "$MOCK_LOG"
  fi
  shift
done
exec "$@"
''')
        self.env = os.environ.copy()
        self.env.update({
            "PATH": f"{self.bin}:{os.environ['PATH']}",
            "MOCK_LOG": str(self.log),
            "MOCK_PGPASS_OBSERVED": str(self.root / "pgpass-observed"),
            "MOCK_EXPECT_PASSWORD": "p@ss:word",
            "AJENT_DNS_BROWSER_ADMIN_DSN": "postgresql://postgres:p%40ss%3Aword@127.0.0.1:55439/ajent_browser?sslmode=disable&dbname=ignored",
            "AJENT_DNS_BROWSER_RUN_ID": "CI-42.1",
            "AJENT_DNS_BROWSER_ARTIFACT_DIR": str(self.root / "artifacts"),
            "PGPASSWORD": "ambient-password-must-not-leak",
        })

    def _command(self, name, body):
        path = self.bin / name
        path.write_text("#!/usr/bin/env bash\nset -euo pipefail\n" + body)
        path.chmod(0o755)

    def _run(self, **overrides):
        env = self.env.copy()
        env.update(overrides)
        result = subprocess.run([str(SCRIPT)], text=True, capture_output=True, env=env)
        self.assertEqual(list(SCRIPT.parent.parent.glob(".dns-browser-private.*")), [])
        observed = self.root / "pgpass-observed"
        if observed.exists():
            pgpass_path = Path(observed.read_text().splitlines()[0])
            self.assertFalse(pgpass_path.exists())
            self.assertFalse(pgpass_path.parent.exists())
        return result

    def test_uses_fresh_loopback_database_installs_browser_and_drops_database(self):
        result = self._run()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.log.read_text()
        database = "ajent_dns_browser_ci_42_1"
        admin_dsn = (
            "postgresql://postgres@127.0.0.1:55439/ajent_browser?sslmode=disable"
        )
        self.assertIn(f"createdb --maintenance-db={admin_dsn} --template=template0 {database}", calls)
        self.assertIn("agent-browser install", calls)
        self.assertIn(
            f"dropdb --maintenance-db={admin_dsn} --if-exists {database}", calls
        )
        self.assertIn(
            "go test -race -json -tags=dnsbrowser ./internal/web -run "
            "^TestBrowserMatchingDNSProof$ -count=1",
            calls,
        )
        self.assertIn(f"DSN postgresql://postgres@127.0.0.1:55439/{database}?sslmode=disable", calls)
        self.assertEqual(calls.count("package --package=agent-browser@0.38.2"), 2)
        self.assertIn("go test -race -json -tags=dnsbrowser", calls)
        self.assertIn("secret-environment=clean", calls)
        self.assertNotIn("p@ss:word", calls)
        self.assertNotIn("p%40ss%3Aword", calls)
        self.assertTrue((self.root / "artifacts" / f"dns-browser-{database.removeprefix('ajent_dns_browser_')}.jsonl").is_file())
        pgpass_path, mode, dir_mode, valid, clean_env = (
            self.root / "pgpass-observed"
        ).read_text().splitlines()
        self.assertEqual(mode, "600")
        self.assertEqual(dir_mode, "700")
        self.assertEqual(valid, "1")
        self.assertEqual(clean_env, "1")
        self.assertFalse(Path(pgpass_path).exists())

    def test_rejects_non_loopback_before_creating_database(self):
        result = self._run(
            AJENT_DNS_BROWSER_ADMIN_DSN=(
                "postgresql://postgres:test@db.internal:5432/ajent_browser"
            )
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("numeric loopback", result.stderr)
        self.assertFalse(self.log.exists())

    def test_rejects_uri_routing_override_before_creating_database(self):
        result = self._run(
            AJENT_DNS_BROWSER_ADMIN_DSN=(
                "postgresql://postgres:test@127.0.0.1:5432/ajent_browser"
                "?host=remote.example&sslmode=disable"
            )
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported connection parameter(s): host", result.stderr)
        self.assertFalse(self.log.exists())

    def test_cleans_up_database_when_browser_test_fails(self):
        result = self._run(MOCK_GO_MODE="fail")
        self.assertNotEqual(result.returncode, 0)
        calls = self.log.read_text()
        self.assertIn("go test -race -json -tags=dnsbrowser", calls)
        self.assertIn("dropdb --maintenance-db=", calls)

    def test_rejects_go_test_run_with_no_tests(self):
        result = self._run(MOCK_GO_MODE="empty")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("requires the named suite", result.stderr)
        self.assertIn("dropdb --maintenance-db=", self.log.read_text())

    def test_rejects_skipped_named_test(self):
        result = self._run(MOCK_GO_MODE="skip")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("skips=1", result.stderr)
        self.assertIn("dropdb --maintenance-db=", self.log.read_text())

    def test_rejects_failure_event_even_if_go_exits_zero(self):
        result = self._run(MOCK_GO_MODE="failed-event")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("failures=1", result.stderr)
        self.assertIn("dropdb --maintenance-db=", self.log.read_text())

    def test_rejects_unpinned_agent_browser_version(self):
        result = self._run(AJENT_AGENT_BROWSER_VERSION="latest")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("pinned semantic version", result.stderr)
        self.assertFalse(self.log.exists())

    def test_uses_executable_browser_override_without_downloading_browser(self):
        result = self._run(AGENT_BROWSER_EXECUTABLE_PATH=str(self.browser))
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.log.read_text()
        self.assertNotIn("agent-browser install", calls)
        self.assertIn("go test -race -json -tags=dnsbrowser", calls)
        self.assertIn("PASS TestBrowserMatchingDNSProof", result.stdout)

    def test_rejects_non_executable_browser_override_before_database_creation(self):
        result = self._run(AGENT_BROWSER_EXECUTABLE_PATH=str(self.root / "missing-browser"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("must name an executable browser", result.stderr)
        self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
