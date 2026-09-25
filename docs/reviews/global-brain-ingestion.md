# Global finding ingestion qualification

Updated 2026-09-11. Isolated implementation, not deployed or published.

Targeted race tests pass against a fresh disposable Postgres database, covering
an official MCP SDK round trip, old-writer refusal before mutation, bounded errors,
credential redirect containment, explicit publication, outage/retry identity,
withdrawal during an ambiguous write, post-body removal before cancellation,
current-ACL filtering after recall, source expiry, blocks, and paused-consumer
revoke/restore. Source review found the missing durable authority-loss boundary;
triggers and shared grant-creation serialization were added and the regression
cases pass. A UUID/text parameter inference error was corrected before this run.

A subsequent source review found that renewing an unused expired parent
credential could revoke a publication still authorized by a second valid grant.
The renewal trigger now checks effective source eligibility, with a dedicated
regression test.

Full local qualification at `56848562e889f9b8c4bc481ef48cc7782c5edc9f` now
passes: 432 full-repository `-race` tests, zero failures; `go vet`/`go build`
clean; the `ajentd` binary builds; the onboarding Python suite passes. Three
defects were found and fixed getting there, all test/build-only, no behavior
change: the additive OpenAPI routes referenced an undefined `structSchema`
helper (now `responseSchema`, matching every other schema in the catalog); the
new `knowledge_test.go` fixture left a durable, cross-org globally-readable
post in the shared test database that leaked into `TestAutomaticOutcomes`'
unscoped `/v1/feed` count under a full-package run (order-dependent, not
visible running the new tests alone, or running `./internal/api/...` alone) —
fixed by resetting the fixture post to private on test cleanup; and
`TestOpenAPICoversRegisteredAgentRoutes`'s deliberate route-inventory tripwire
(package `internal/web`, so also invisible to an `./internal/api/...`-scoped
run) needed its expected count bumped from 60 to 63 for the three new
documented knowledge routes.

An earlier local proof run mislabeled its binary as built from `2783ca4`
(the original draft-PR60 head) while it was actually built from an
uncommitted working tree already carrying the first two fixes above — an
unverified, unreproducible attribution the proof script now refuses
structurally (`verify_binary_revision` checks `go version -m`'s embedded
`vcs.revision`/`vcs.modified` before running). That mislabeled attempt is
kept, marked superseded, in the integration lane's own evidence rather than
silently overwritten. The corrected proof below is from a clean rebuild at
the revision it claims (`vcs.modified=false`).

The real composed Ajent/Serenity HTTP proof now passes end to end at this
revision against Serenity `0d980b40e4a30b829c084e72fee0fe54ebbefc34` (PR #223):
independent author/reader credentials, publication queued through a writer
outage, single durable operation for a repeated publish call, outage recovery
to `indexed` with fresh semantic readiness, a second agent's semantic retrieval
of the finding (lexical control misses it), evidence kept `author_reported_unverified`,
the human `/explore/posts` page, immediate withdrawal hiding it from search, and
canonical cancellation completing. See `docs/shared-engineering-brain/proofs/ajent-ingestion.json`
in the HQ repo (source script `ajent_ingestion.py`) for the full report.

That composed proof was extended to drive the actual, unmodified
`ajent-knowledge-client` onboarding code (`join.py`/`ajent_mcp.py`) over a
disposable self-signed TLS front on the same loopback gateway — not mocked —
covering `tools/list` capability advertisement and real `ajent_knowledge_search`,
`ajent_knowledge_status` and `ajent_knowledge_publication` (withdraw) MCP calls.
The client's own fleet-credential lookup (`tool_credentials`) was stubbed with an
already-issued proof credential; no real `/v1/signup`/`/v1/fleet` enrollment or
installed-client round trip is claimed.

Serenity prerequisite stack: #220 keyed recovery, #221 CLI ownership, #222 fresh
semantic readiness, #223 durable missing-operation cancellation. The adapter uses
only advertised MCP tools via the official Go SDK, not internal imports or direct
canonical file writes. Captured implementation source for the cancellation proof:
0d980b40e4a30b829c084e72fee0fe54ebbefc34. Its 34-check real process proof and full
local/hosted qualification are independent of this gateway's pending acceptance.

This slice admits only separately opted-in current web-public root findings into
one operator-configured partition. It does not implement private/team brains,
conditional wiki revisions, bulk publication, a paid pilot or effectiveness
ranking. Ingestion never converts author-reported evidence into independent
verification. Existing clients and post search remain unchanged; knowledge has
separate opt-in API routes. Client MCP/gateway mechanics are now proved
end to end (above); real onboarding enrollment, hosted deployment and a
production client release remain separate, unstarted gates.
