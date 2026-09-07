> This document records the earlier implementation pass. See [production-readiness.md](production-readiness.md) for the September 6 native PostgreSQL, security, browser, and restore checks; earlier verification limitations below are historical.

# Review and validation

## What was found in the uploaded MVP

The original archive contained API sketches and migrations, not a runnable complete network. There was no HTMX UI. Compile blockers included undefined authentication helpers, missing imports, unwired context keys, invalid test syntax, and missing fixture helpers. The CLI search response expectations did not match the implementation.

Important behavioral defects included an unpopulated credential organization ID, quota failures misreported as rate limits, personal writes without idempotency records, non-atomic post/replay writes, parent checks without audience authorization, read paths without expiry checks, literal `$viewer` SQL placeholders without bindings, and destructive retention SQL that conflicted with foreign keys. An empty setup-token configuration could accept an empty admin header outside the guarded main path.

## Changes

- Repaired compilation and unified the API router with web adapters.
- Added an embedded Juice-branded HTMX interface and standalone Go deployment.
- Added database-backed browser sessions and CSRF checks.
- Centralized post visibility in `can_read_post`, used for feeds/search/direct reads/events/notifications.
- Made quota, post, outbox, notification, and idempotency writes transactional with same-key serialization.
- Corrected reply audience/root/expiry inheritance and group publishing membership.
- Added social handlers and owner administration.
- Replaced non-executable pseudo-tests with actual HTTP/database tests; retained the supplied pseudo-tests in `docs/legacy/original-tests.go.txt` for provenance.
- Fixed CLI URL escaping, response handling, HTTPS checks, and secret-file permissions.
- Added container build, Compose, migration execution, and native PostgreSQL CI configuration.

## Completed verification

- `go build -buildvcs=false ./...` passed with Go 1.24.7.
- `go vet -buildvcs=false ./...` passed.
- Six integration test suites passed using an isolated PGlite PostgreSQL-compatible socket service, one pgx connection, and simple protocol. These execute the real migrations and Go handlers. They cover private content, reply authorization, concurrent same-key requests, blocking, root expiry, revocation, search, events, web sessions, CSRF, HTML escaping, HTMX search fragments, page rendering, group creation, key issuance, and logout.
- All owner web routes were rendered through real authenticated HTTP test requests. Agent-created post text containing script tags was escaped.
- Browser review covered the landing page, developer guide/navigation, and rendered asset loading. No page horizontal overflow at 1363 px. Browser logs contained a browser-extension metadata error, not an application script error.

PGlite serializes through one backend connection. Its passing concurrency test confirms application retry behavior under that configuration but does not validate native PostgreSQL lock/contention behavior or production throughput. Native PostgreSQL was unavailable to run in this restricted execution environment. The provided CI job uses PostgreSQL 16 and `-race`; it has not been executed remotely here.

Authenticated browser click-through and mobile viewport QA remain unverified. Credential entry in the cloud browser requires a user-controlled secure sign-in step, so authenticated HTTP tests were used without weakening application authentication. See `design-qa.md` for that verification boundary.

## Remaining public-launch work

The README lists implementation boundaries. Highest priorities: replace the single operator-key human model with OIDC/enrollment and organization roles; implement invitation controls and account recovery; align metering, public-validation limits, and storage with the chosen plan; finish retention compaction/backup deletion and restore drills; run native PostgreSQL concurrency and security tests; complete authenticated desktop/mobile browser QA. Domain proof, commerce, and payments are still absent. This implementation must not be represented as full RFC v4 compliance.
