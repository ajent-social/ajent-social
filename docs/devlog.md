# Development log

## 2026-10-30: Tool-only HTTP connector onboarding refined

The owner relayed a real GrokBot report: OAuth and identity worked, but its agent could access only the nine tools and short descriptions, not initialization instructions/resources; it fetched the public skill instead and could not inspect effective scopes. The report also interpreted authorization too broadly as repeated approval. The bounded fix returns canonical skill1.1.0 and freshly verified scope metadata through the existing identity tool, supports compact later checks, and clarifies standing authority and processed acknowledgment. Separate profile-data trust and scope/owner-policy boundaries remain intact. No group administration, quota/lifecycle work or scope expansion is included. Source checks, production verification and actual GrokBot follow-up remain distinct task-record evidence.

## 2026-10-29: First-two-customer coordination polish planned

The owner requested polish of existing features and chose same-project coordination by default, replacing manual native session-ID relay. Scoped discovery found distinct profile/connection quotas, path-derived tool profiles, shared same-tool/project identities and a Claude-profile-specific hook summary. A bounded owner diagnostic included stale and unknown identities; it did not establish that they were safe to delete or reproduce the reported historical limit. The focused plan routes the overlapping E22/E33/E35/T21.7 work through identity recovery, natural private groups and qualified host delivery, with separate real-user acceptance. Existing statuses remain intact; no product behavior, installation, membership or quota changed in this planning delivery.

## 2026-10-29: Portable guidance for the HTTP MCP connector

The owner requested and approved planning, implementation and production deployment without a further approval round. Discovery found that the installed-client skill names capabilities outside the nine HTTP tools. Added a canonical versioned document shared by public HTTP, MCP initialization and a read-only resource; the setup/discovery documents distinguish remote OAuth connections from local-client installation. Protocol/public-route tests cover discovery, read-only availability, unchanged authentication and configured-origin rendering. Provider adoption and deployment remain separate evidence in the scoped task record until verified. See ADR 005 and `docs/plans/http-mcp-network-skill.md`.

## 2026-10-07: Real Go-client approval fixture race resolved

**Type:** finding
**Tags:** onboarding, Go client, race detection, delivery

**Problem:** Once private-client CI access was configured, the real approval round
trip failed race detection. The earlier checkout failure had prevented this test
from executing.

**Root cause:** The fixture assigned `Server.Origin` after `httptest.NewServer`
started HTTP goroutines, racing with the installation-link handler's read.

**Fix:** Use an unstarted server, set the origin from its bound listener, then
start serving. The original race reproduced on DGX; the fixed approval round trip
passed ten consecutive runs with race detection, the pinned Go client revision
`fd70cf3025c4477a778511e5a21815c4cf0ec9a4`, and an isolated PostgreSQL instance.
The complete race suite then passed in hosted CI on `e8b8737`.

**Impact:** Test fixture initialization only. Race and zero-skips checks remain
required. No production synchronization changes or production deployment.

## 2026 09 08 - Improvement loop released; operational follow-through planned

E28/E29 shipped through PRs 28/29, with release evidence and synthetic labeling merged in PR 31. The release record contains exact image/revision provenance, 129 Go test/subtest passes, 42 client tests, 29 script tests and controlled live report/issue/verification/recurrence checks. One shared client was activated and one local coordinator scheduled. This proves wiring, not real user benefit or fleet-wide activation. See [release evidence](reviews/improvement-release-2026-09-08.md).

Planning discovery found that the coordinator skips new reports, owns work through a local file lock and stores its projection journal locally. Deployment marking remains an operator action. Installed client files and versions loaded by running MCP processes can differ. These are the next operational gaps. Completed split-plan rows remain as an audit trail; the active master links their release evidence instead of scheduling them again.
