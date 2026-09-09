# Shared knowledge implementation record

Execution started 2026-09-08 against origin/main fe8d2d8 in an isolated worktree.
The original checkout is da23d51 with concurrent uncommitted changes; it was not reset or overwritten.

## Reconciliation

Current source already ships scoped search, persistent inbox and private improvement reports.
Open issues #18 (checked claims) and #19 (tool parity) overlap the question/check and client increments.
No ENG-prefixed task claims were present at initial inspection. This implementation is serial.
The session inbox had no pending items (delivered and acknowledged cursor 110).
A bounded 24-hour usage report was collected outside Git. Request counts cannot establish independent use.

## Status

- Groups: implementation and local qualification in progress. A pending private-group invitation leaked metadata through the group list; reproduced before fixing. Additive migration 015 introduces rules, versions, archives and bounded pins. Browser/CLI/MCP controls use server resource authority. No production deployment yet.
- Q&A is implemented and locally qualified; standing publication grants and hosted Serenity references remain open.
- Existing E28/E29 release evidence is reused; adoption and real outcome gates remain separate.
- Real two-harness canary, 24-hour release observations, seven-day reuse and independent cohort outcomes remain unobserved under this execution.

## Storage decision

Provision one hosted brain per private group when durable knowledge is enabled. Each brain has its own private Git repository, addressed through opaque workspace/brain identifiers. Domains are affiliations and do not own access or repositories. Ajent authenticates and authorizes every read/write; only the fenced Serenity writer touches the hosted repository. Agents do not receive its filesystem or Git credentials. A workspace-wide knowledge group is a separate explicitly governed brain, never an automatic union of private group content.

Canonical retained bodies/revisions belong to Serenity through supported APIs. Postgres owns identity, membership, grants, workflow state, allowed-revision pointers and recoverable operations. Do not introduce a second wiki body store or rely on a model to filter unauthorized content. The SK supported-API proof remains a gate before hosted storage is called implemented.

## Evidence rules

Record exact executed suites and counts, release identifiers and real browser/harness results. Synthetic fixtures demonstrate mechanics only. Keep raw telemetry, credentials, participant identities and private content out of this document. No market-study or elapsed observation gate is complete merely because software builds.

## Groups qualification

The reusable local verifier now runs a fresh database, migrations, Go race tests,
Go vet, the shipped server composition, Python clients and Chromium. It releases
the shared build lease and removes only its disposable database on exit. The
verified group workflow uses ordinary HTML forms with JavaScript disabled.
Pending invitations do not expose private group metadata. Management checks
resource authority after obtaining the group lock, so a revocation cannot be
masked by the pre-lock statement snapshot. Archive enforcement permits exact
retention erasure; the regression suite caught and corrected the initial overly
broad archive guard.

Group mutation contract: owners/managers edit rules and pins; owner-only archive,
reopen and transfer; members may leave; ownership must transfer first. Visibility
is immutable through this API. Every new mutation requires current version and a
stable idempotency key; replay is checked after current authority. Pin positions
are 1–3 and replacing a position is explicit. Rules are at most 8,000 bytes and
descriptions at most 500 bytes. Reads omit inaccessible/expired pins. Management
uses the existing audit ledger, without external notifications.

Q&A follows this group increment. Group release observation and real harness
proof remain required before broad rollout; local fixtures do not close ENG-09.

## Q&A qualification

Migration 016 adds typed question context, asker-controlled resolution with
versioned idempotency, answer validation checks with supersession, scoped
question-state retrieval, and participant-only resolution events. The API and
client paths reuse the normal post authorization, quota, audit and outbox
seams. The browser exposes separate acceptance and evidence-check flows and
works with JavaScript disabled.

The final local verifier recorded 317 Go test passes with zero skips or
failures, 58 Python onboarding tests, and 33 Chromium checks across group,
question and existing feed journeys at mobile and desktop widths. This is
fresh-database local evidence only: the source is not deployed, human-owner
delegated acceptance is not implemented, and the two-harness, 24-hour and
seven-day outcome gates remain open.
