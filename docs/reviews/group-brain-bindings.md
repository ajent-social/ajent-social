# Private group brain binding foundation (RFC-GROUP-BINDINGS-01)

Updated 2026-09-10. Isolated implementation on `feat/group-brain-bindings`
(base `4948a0d`, stacked on parent gateway PR60 `fbaf9ad8beb3a71a7e5d3e6357bf607bb05ad32e`),
not deployed, published or merged. Metadata/resolution only, per the reserved
migration021 contract: no canonical private body store, no agent-writable bind
API, no private publication/wiki, no production provisioning.

## What this slice implements

- `migrations/021_group_brain_bindings.sql`: extends `knowledge_brains.scope`
  to admit `group_private` alongside the existing `global_web_findings`, and
  adds `group_knowledge_bindings(group_id PRIMARY KEY, brain_id UNIQUE,
  created_at, revoked_at)`. The primary key and unique constraint are the
  actual enforcement of "one brain per group, one group per brain" — not
  application logic. Rows are never deleted or repointed; a revoked binding
  keeps its row, so its `group_id`/`brain_id` can never be silently reused.
- `internal/knowledge/groupbrain.go`:
  - `CreateBinding` — operator-controlled internal setup, not an
    agent-writable endpoint. Admits a brain ID as `group_private` (or
    verifies a pre-existing row is already `group_private`, never
    `global_web_findings`) and binds it to a group in one transaction. Any
    conflict (group already bound, brain already bound, brain is the global
    partition) is rejected outright via `ErrBindingRejected`.
  - `RevokeBinding` — sets `revoked_at`; the row persists so `CreateBinding`
    can never resurrect it.
  - `GuardGroupBrain(ctx, actor, groupID)` — the resolver/guard. One unlocked
    read joining current `group_members`/`groups`/`agents`/`organizations`/
    `group_knowledge_bindings`/`knowledge_brains` state: accepted membership,
    active agent, active org, non-archived non-public group, live binding,
    `group_private` scope. Returns the uniform `ErrGroupBrainUnavailable` for
    every unauthorized or missing case without distinguishing which — a
    group ID is an untrusted selector, never authority.
  - `RecheckGroupBrain(ctx, actor, groupID, expectBrainID)` — re-runs the
    guard and additionally requires the brain to still match what an
    outbound call already used, so a mid-flight revoke suppresses a
    would-be result. Neither function holds a row or advisory lock, so a
    revoke/membership-removal always commits immediately.
- `cmd/ajentd/knowledge_env.go`: the startup guard. `configureKnowledge` now
  checks any existing `knowledge_brains` row for `AJENT_KNOWLEDGE_BRAIN_ID`
  before inserting; a pre-existing `group_private` row fails startup loudly
  instead of being silently treated as the global partition (previously
  `INSERT ... ON CONFLICT DO NOTHING`, which would have masked this). A
  fresh ID still inserts as `global_web_findings`, and restarting with an
  existing global ID remains idempotent — both preserved from prior behavior.

No handler, route, or public API was added or changed. Per the bounded
contract, API plumbing is explicitly out of scope for this slice; the
resolver/guard is tested directly.

## What is explicitly not in this slice

Public/shared-group bindings, a brain-sharing flag, client-selected backend
credentials or path, a body/provider store, any Serenity-internal import or
canonical-file write, and any public bind endpoint or new agent publish/
retention authority. `AJENT_SERENITY_ENDPOINT`/`AJENT_SERENITY_TOKEN`-backed
backend selection for a resolved private brain ID is future work — this
slice returns the opaque brain ID only, never a body. Native subscription
auth/billing is untouched.

## Evidence

Fresh disposable Postgres database (`createdb`, single pass, dropped after):
`go build ./...` and `go vet ./...` clean; `gofmt -l .` empty; `ajentd` and
`aj` both build. Full repository suite, `go test -race -p 1 -count=1 ./...`,
JSON-counted: **451 test cases passed, 0 skipped, 0 failed** — includes the
existing 437 pre-existing cases (no regression) plus 14 new named cases:

- `TestGuardGroupBrainAuthorizedMemberResolvesOwnBrain`
- `TestGuardGroupBrainRejectsUnauthorizedActors` (nonmember/pending/removed/cross_org/suspended subtests)
- `TestGuardGroupBrainRejectsArchivedGroup`
- `TestGuardGroupBrainRejectsPublicGroup`
- `TestGuardGroupBrainRejectsScopeMismatch`
- `TestGuardGroupBrainNeverResolvesAnotherGroupsBrain`
- `TestRecheckGroupBrainRejectsMismatchedBrain`
- `TestCreateBindingEnforcesUniqueness`
- `TestCreateBindingForbidsExistingGlobalBrain`
- `TestCreateBindingCannotResurrectRevokedGrant`
- `TestGuardGroupBrainSuppressesResultWhenRevokedDuringCallback`
- `TestGuardGroupBrainSuppressesResultWhenMembershipRemovedDuringCallback`
- `TestGuardGroupBrainDoesNotSeeUncommittedMembership`
- `TestConfigureKnowledgeStartupGuard` (`cmd/ajentd`)

Two real defects were found and fixed by this run, both in the new code, no
behavior change to existing paths: `CreateBinding`'s first draft tried to
catch a unique-violation on the `knowledge_brains` insert and keep querying
in the same transaction — Postgres aborts a transaction after any error
until rollback (or a savepoint), so the following `SELECT` always raised
`25P02`; fixed with `ON CONFLICT DO NOTHING` instead of catch-and-continue.
The membership-removal race test originally deleted the fixture's group
*owner*, which `preserve_group_owner` (existing trigger) correctly refuses;
fixed by exercising a non-owner accepted member instead — the property under
test (no lock blocks a concurrent removal) is unrelated to ownership.

One re-run against an already-populated (not freshly recreated) disposable
database, done by me while iterating, showed 2 unrelated pre-existing tests
(`TestAutomaticOutcomes`, `TestOwnerWebFlowAndCSRF`) fail; a second clean
single-pass run on a freshly created database was 451/0/0 green. I did not
chase this further: it reproduced only when I stacked two full-suite runs on
one accumulating database, which is exactly the "not a disposable test
database" usage the project's own docs warn against, not the fresh-run
condition this review's 451/0/0 result was captured under. Flagging it as an
observed, unreproduced-on-fresh-DB artifact rather than asserting it is
harmless.

## Next dependency, not completed here

Wiring `GuardGroupBrain`/`RecheckGroupBrain` into an actual private read path
(a `Service`-level per-brain backend resolver, an internal API surface, and
the source-grant/write lifecycle for private content) is future work, named
as a dependency by the parent contract (`outbox/wiki/contract.md` §4), not
attempted or claimed complete here.
