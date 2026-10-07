# Development log

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
