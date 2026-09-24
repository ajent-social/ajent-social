# Local changes and PR maintenance — 2026-09-24

## Scope and baseline

Reviewed the original dirty checkout at `da23d516e744`, current service main
at `eaf184e`, all eight open PRs, adjacent client main at `c30235f`, and the
skills catalog extraction. This is maintenance and local verification, not a
deployment or launch approval. Existing client/discovery sessions remain active.

## Local archive

Retained the September 15 reconciliation and its compatible CSS changes in
PR #75. A fresh private archive under Git metadata preserves 439 working files,
SHA-256 manifest, binary working/index patches, status/worktree inventories and
an all-ref Git bundle. Every archived file hash and the bundle were verified.
Nested worktrees and generated caches are excluded. The original checkout is
preserved; local configuration and raw evidence are not published.

The requested Bugbot review found no additional actionable local bugs after
checking blob/history equivalence against main. Replaying older backend,
telemetry, reliability, client and UI changes would regress newer work.

## PR disposition

| PR | Disposition and evidence |
| --- | --- |
| #60 | Consolidated into #68, including its unique reconciliation merge `8200120`; archive after the consolidated head is published. |
| #61 | Closed as superseded; exact head `8f096f8` is an ancestor of #68. |
| #62 | Closed as superseded; exact head `1903d80` is an ancestor of #68. |
| #66 | Closed as superseded; exact head `24b22d1` is an ancestor of #68. |
| #68 | Retain the consolidated knowledge candidate; reconcile current main and fix private-worker starvation and global capability/route handling in private-only configurations. Remains draft. |
| #70 | Retain: direct-application analytics is absent from main. Reconcile current public copy/navigation and retain its public-view allowlist. |
| #75 | Retain compatible guide/directory CSS and this maintenance/ownership record. |
| #77 | Closed: catalog belongs to `ajent-social/skills`, per client ADR 008 and the service channel decision. `internal/skillcatalog/catalog.go` is byte-identical to the extracted repo. Its unrelated older dashboard must not overwrite current main. |

Closed PR branches and their evidence are retained. Closure means superseded,
not merged, shipped, or proven in production.

## Client and discovery boundary

Client ADR 008 makes the Go CLI/MCP/installers/integrations and public agent
materials canonical in `ajent-social/ajent`; the catalog lives in
`ajent-social/skills`. The client E8 launch/cutover remains incomplete. Keep the
service's current Python compatibility code until the staged identity-preserving
cutover is qualified. The old local-memory plan now calls out transferred client
work without claiming its memory requirements complete. Active private discovery
contracts and Gist reuse work are not obsolete and were left with their owners.

## Verification and limitations

Both #68 defects have regressions observed failing before the fixes and passing
afterward. The outbox test covers both idle indexed rows and terminal operation
conflicts hiding another brain's pending work. Private-only configuration tests
cover capability advertisement and all four global routes; the old search path
panicked on a nil backend. Exact final suite results are recorded on #68.

The reconciled #68 Python onboarding suite passed 84 tests. The reconciled #70
analytics and rendered-template fixture tests passed, as did all 10 Worker tests.
#75 retains its prior browser evidence; this pass does not claim a fresh browser
or production GA4 check.

The bounded telemetry window was 2026-09-23 22:01:30 to 2026-09-24 22:01:30 UTC:
28,691 requests, no recorded panics, 3,594 application requests attributed to
release `e2981ba780d461e5a218759c830df25232ab39c3`. The aggregate report is local,
not committed. These are requests, not users or successful workflows. Absence of
panics does not qualify unreleased candidate code.
