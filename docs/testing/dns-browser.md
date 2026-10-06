# Isolated DNS proof browser verification

`internal/web/dns_browser_test.go` runs the real web routes, database migrations,
sessions, CSRF validation and domain forms for deployment owner, original unlinked
installation credential, and human account owner. Each role creates a challenge,
receives a wrong TXT response (409, actionable recovery, no verified row), then
receives the exact displayed proof (303, verified UI after reload, matching
workspace/proof token, more than 29 days validity and a scheduled next check).
The test also asserts the exact TXT question and configured response.

The file uses the `dnsbrowser` Go build tag, so the test is absent from ordinary
test runs and cannot add an intentional skip to the default zero-skips gate.
Run it by itself because it replaces process-global resolver and setup-key
state. The wrapper creates a unique empty database whose name starts with
`ajent_dns_browser_`, installs the pinned `agent-browser` CLI, reuses a supplied
browser executable or installs its browser, runs this test alone, then drops
the database on success or failure.

It requires `createdb`, `dropdb`, `go`, `npm`, and `python3` on `PATH`, plus a
PostgreSQL URI with create/drop permission and numeric loopback host `127.0.0.1`.
The wrapper rejects DNS names, IPv6, and remote database hosts before creating
anything. Its URI query allows only `sslmode`, `connect_timeout`,
`application_name`, and optional `dbname`/`database`; database-name query values
are removed when it constructs the test DSN. The UI CI job should add this step
after its existing Node and browser setup:

```sh
AGENT_BROWSER_EXECUTABLE_PATH="$(node -p 'require("playwright").chromium.executablePath()')" \
AJENT_DNS_BROWSER_ADMIN_DSN="${AJENT_DNS_BROWSER_ADMIN_DSN:?set a loopback admin URI}" \
scripts/test-dns-browser.sh
```

The wrapper reads the source URI from its environment, then removes it before
starting child commands. It passes only passwordless URIs to PostgreSQL tools
and Go, places the decoded credential in a mode-0600 `pgpass` file inside a
mode-0700 task-private directory on the worktree volume, and removes that
directory on success, failure, or handled termination.

The UI job should use Node 24 or newer for `agent-browser` 0.38.2. The wrapper
pins that CLI version and downloads its browser only when
`AGENT_BROWSER_EXECUTABLE_PATH` is unset. When set, the path must be executable;
the existing Playwright Chromium path avoids a second browser download on the
Linux ARM64 runner. Override the CLI version only with an explicit semantic
version through `AJENT_AGENT_BROWSER_VERSION`. The wrapper chooses a per-run
database name; a collision fails safely instead of reusing or dropping an
existing database. It runs the tagged test with `-race -json`, saves the event
stream under `artifacts/verification/`, and requires the named suite and all
three role subtests to run and pass with zero skips or failures. Offline tests
in `scripts/test_dns_browser_script_test.py` cover database routing, cleanup,
omitted tests, skips, failure events, and an executable browser override.

Historical evidence on 2026-09-08, base `40de0f9`: the prior manual command with fresh database
`ajent_dns_browser_race2` executed one named suite and all three role subtests;
six wrong/matching outcome checks passed, with nine actual domain form submits.
Race detector reported no races. Retained session evidence:
`/tmp/ajent-dns-browser-race2.log` and the audit scratch artifact
`ui-dns-browser-probes.json`. All named browsers and the HTTP listener closed;
the resolver was restored and disposable databases were dropped after readback.

DNS is a test-only allowlisted responder over `net.Pipe`, consumed by Go's actual
`net.Resolver.LookupTXT` DNS stream parser. No DNS listener or upstream query is
created; HTTP binds only `127.0.0.1`. No host resolver settings or production hooks
change. This proves form/handler/TXT interpretation, not DNS propagation,
authoritative DNS availability, UDP/TCP transport, TLS or production deployment.
The in-memory stream relies on Go resolver `net.Conn` framing behavior.

Owner and original credentials use the real login form. The human session is a
synthetic fixture, so this test does not prove passkey authentication; the real
WebAuthn browser suite supplies that separate evidence. No verified domain rows
are seeded. Scoped grants and agent assignment have separate audit probes.
