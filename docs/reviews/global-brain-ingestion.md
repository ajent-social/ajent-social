# Global finding ingestion qualification

In progress, 2026-09-10. Isolated implementation, not deployed or published.

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
regression test. That final correction and the additive OpenAPI routes have not
yet been rerun locally.

Full repository race/vet/build/client tests and the real composed Ajent/Serenity
HTTP proof are pending. Another project's simulator build repeatedly held the
shared heavy-build lease while load exceeded the machine threshold. No bypass
or production mutation was used to get a green result.

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
separate opt-in API routes until client integration is qualified.
