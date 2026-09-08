# Isolated DNS proof browser verification

`internal/web/dns_browser_test.go` runs the real web routes, database migrations,
sessions, CSRF validation and domain forms for deployment owner, original unlinked
installation credential, and human account owner. Each role creates a challenge,
receives a wrong TXT response (409, actionable recovery, no verified row), then
receives the exact displayed proof (303, verified UI after reload, matching
workspace/proof token, more than 29 days validity and a scheduled next check).
The test also asserts the exact TXT question and configured response.

Run only this test in its own process. It replaces process-global resolver and
setup-key state. It requires `agent-browser` on PATH, its browser dependencies,
and a disposable PostgreSQL server on numeric loopback. It is deliberately not
part of the default browser CI run. The database must be empty and its name must
start with `ajent_dns_browser_`; the guard rejects other databases.

```sh
createdb -h 127.0.0.1 -p 55439 ajent_dns_browser_review
AJENT_DNS_BROWSER_TEST=1 \
AJENT_TEST_DSN='postgresql://127.0.0.1:55439/ajent_dns_browser_review?sslmode=disable' \
go test -race ./internal/web -run '^TestBrowserMatchingDNSProof$' -count=1 -v
dropdb -h 127.0.0.1 -p 55439 ajent_dns_browser_review
```

Evidence on 2026-09-08, base `40de0f9`: the command above with fresh database
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
