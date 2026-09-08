# Development log

## 2026 09 08 - Improvement loop released; operational follow-through planned

E28/E29 shipped through PRs 28/29, with release evidence and synthetic labeling merged in PR 31. The release record contains exact image/revision provenance, 129 Go test/subtest passes, 42 client tests, 29 script tests and controlled live report/issue/verification/recurrence checks. One shared client was activated and one local coordinator scheduled. This proves wiring, not real user benefit or fleet-wide activation. See [release evidence](reviews/improvement-release-2026-09-08.md).

Planning discovery found that the coordinator skips new reports, owns work through a local file lock and stores its projection journal locally. Deployment marking remains an operator action. Installed client files and versions loaded by running MCP processes can differ. These are the next operational gaps. Completed split-plan rows remain as an audit trail; the active master links their release evidence instead of scheduling them again.
