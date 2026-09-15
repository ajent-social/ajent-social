# Local-work reconciliation — September 15, 2026

Compared the dirty checkout at `da23d516e744` with freshly fetched `origin/main`
at `9ce85fcd5df5`. This is a source reconciliation, not a deployment report.
The original checkout was preserved. A private local archive stores the working
files, binary working/index patches, status, worktree inventory and an all-ref
Git bundle. Local configuration and raw evidence are not published in this PR.

## Inventory and interpretation

| Classification | Entries |
| --- | ---: |
| Byte-identical to current main | 49 |
| Three-way merge proves local changes incorporated in current main | 2 |
| Exact local blob reachable in main history, now superseded | 25 |
| Modified tracked files requiring reconciliation | 10 |
| Untracked files with a different version in main | 5 |
| Untracked files absent at the same path in current main | 227 |
| Nested-worktree entries and generated caches excluded | 24 |

These are file/entry counts, not independent features. A file absent from main
is not proof that its described feature is unimplemented. Historical reports
are evidence of their recorded window only. Exact-blob history matching does
not establish that every historical behavior remains desirable today.

## Decisions

- **Reliability, telemetry, clients and deployment:** preserve current main.
  The local implementations are already incorporated or exact older versions
  reachable in main history. No backend or client replay is needed.
- **UI polish:** preserve current page shells, mobile navigation, dark theme,
  focus colors, sign-in recovery and dashboard/goals UI. Older templates would
  remove subsequent improvements. The local health-row class is unnecessary:
  main already styles the same articles and diagnostic definition lists.
- **Retained CSS:** use existing spacing and radius variables for ask cards and
  directory layout; separate standalone directory cards and wrap long inline
  code. Use main's variable names and values rather than restoring the old
  competing design-token set. Main's page-shell width and insets still apply.
- **Planning documents:** retain current completion and acceptance statuses.
  The local groups ADR predates contract acceptance, and the local execution
  index resets completed rows and omits the newer workspace-membership epic.
  Its additional sequencing proposal and the local-memory clarification remain
  in the private backup for a separate planning review; they are not adopted as
  new priorities by this maintenance PR.
- **Local reports, screenshots and marketing drafts:** preserve in the backup.
  Several reports describe historical releases; the launch campaign is marked
  proposed. Do not bulk-import old evidence as current validation or approve a
  campaign by committing it accidentally.
- **Agent configuration, caches and nested worktrees:** preserve locally; do not
  include them in the product PR or change any live integration.

## Validation

- `go test ./internal/web -run '^TestVisualFixtures$' -count=1 -json`:
  49 rendered fixture subtests plus the parent passed; zero skips or failures.
- `node scripts/ui-contrast.mjs` against those newly rendered fixtures:
  490 Chromium checks (49 pages, five widths, light/dark), zero detected
  accessibility violations or horizontal overflow. The existing runner also
  exercised its keyboard/scroll checks.
- Inspected the mobile dark participation page and desktop light guide image.
- `git diff --check` passed.
- Backup verification matched all 455 enumerated working files and verified the
  Git bundle. The original checkout's porcelain status remained unchanged.

Browser checks used synthetic local fixtures, not production accounts. No new
production deployment was performed. Historical UI-polish test results were
not reused as proof for this revision.
