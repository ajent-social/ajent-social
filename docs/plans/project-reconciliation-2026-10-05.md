# Project reconciliation — 2026-10-05

Scope: reconcile in-progress service work, retire superseded candidates, preserve pending planned work and active ownership. Production deployment, new roadmap implementation, account settings and destructive changes to the original checkout are excluded.

## Initial audit evidence and disposition

Base: `1853cf7f975e728865ff25115d88c34f0a89842b` on origin/main. The original checkout at `da23d51` is 253 commits behind and three ahead; it is not the current product. September 24 reconciliation already preserved 439 working files and recorded why replaying the old source would regress current work.

- PR #68 (knowledge and private-worker fixes), #75 (compatible UI polish and maintenance records), and #76 (AMSL wiki) are merged. Their old checklist/branch state is not evidence of unfinished delivery.
- PR #70, head `f69e5b7962c5757410aeafca2cc3f2121351da9f`, is superseded. Its analytics implementation is tree-identical to main; the complete tree differs only because it lacks the subsequently added DNS browser test and its documentation. Independent Luna audit confirmed this. Closed October 5; do not merge the stale candidate.
- PR #80, head `bdaea6a832cc327c601a95c1c58a21d778b7d4d8`, remains active planned Go-installer/Python-client retirement. Remote claim `R-crew-python-service-retirement` is owned by another coordinator; preserve its dirty worktree. Infrastructure, UI and container jobs passed; its test job in [run 37339761974](https://github.com/ajent-social/ajent-social/actions/runs/37339761974) failed cloning the client repository with “repository not found”. This is checkout/access evidence, not an application test result. Its signed-off source, independent review, merge and landed checks remain that lane's responsibility.
- Discovery work has existing P02D/S03 claim refs. Preserve those worktrees and pending branch content; this maintenance task does not take ownership of another agent's work.
- E21–E27 growth/activation/cohort work, E30–E35 operations/fleet work, and E37 membership contain pending plan rows. Preserve them. Unchecked rows are not proof that source is absent or authority to initiate deployment/outreach. Future execution needs task-level reconciliation with current code and evidence.
- Original dirty checkout, local reports, infrastructure configuration, and unresolved unique branch patches are retained. Age, divergence and archive existence alone do not justify deleting authored work.

## Historical delivery checkpoint — 2026-10-06

The owner subsequently granted access and instructed "Fix and merge." PR #80
integration now runs in a separate worktree under the coordinator's own claim;
the previous retirement worktree and claim remain untouched. Candidate
`e28b61c394d1e9a2c80082dc547ea9f89871c535` has independent source approval.
It carries patched OpenTelemetry dependencies, dedicated DNS browser execution
with no skipped selected tests, fail-closed signed-installer readiness and
verification on Unix and PowerShell, and corrected onboarding/browser checks.

[Run 37426828612](https://github.com/ajent-social/ajent-social/actions/runs/37426828612)
has passed service build/vet, both vulnerability scans, infrastructure tests,
installer tests and actual DNS/full browser journeys. The private-client
checkout gate fails because `AJENT_CLIENT_READ_TOKEN` is absent; the ordinary
client-dependent enrollment/race suite has not run. GitHub policy disallows
deploy keys. Supply a read-only token scoped to the client repository through
service Actions secrets; do not transfer a broad personal token or alter policy.

At that checkpoint, PR #80 and the maintenance follow-up remained unmerged.
The following receipt supersedes the access blocker and pending retirement state.

## Retirement delivery receipt — 2026-10-07

The owner supplied the read-only repository secret. The newly executable client
approval test exposed a fixture initialization race, which was reproduced and
fixed on DGX; its real-client race test then passed ten consecutive runs.
Independent review approved final head
`e3e66d2f11152650be6953109108e29e054de398` against the original main base
`1853cf7f975e728865ff25115d88c34f0a89842b` through the recorded full and affected
reviews. [Run 37575654148](https://github.com/ajent-social/ajent-social/actions/runs/37575654148)
passed all four CI jobs, including 564 Go cases with race detection, zero failed
or skipped cases, actual pinned-client approval, infrastructure/security checks,
DNS and browser journeys, the three-engine visual sweep, and container checks.

[PR #80](https://github.com/ajent-social/ajent-social/pull/80) was rebase-merged to
`b6c5e52b539a795ffff8c08681c56f4edb825839`. Landed verification confirmed target
reachability and identical Git trees between the reviewed candidate and landed
revision (`356d6b8d95c9d9c5810c0464dfa19e1575b836e4`), preserving the applicability
of candidate CI evidence. No production deployment or release activation occurred.

PR #81 is refreshed on that landed main and contains maintenance records only.
Its own exact-head review, checks, merge and landed receipts are recorded on the
PR after this commit is created; R4-R6 below do not pre-claim those future steps.
DGX owns active work; original Mac checkouts, dirty work, other agents' claims,
and pending roadmap scope remain preserved.

## Delivery tasks

- [x] R1 Inventory current main, PRs, claims, worktrees and plans. kind: implementation; stage: implement.
- [x] R2 Independently compare and close superseded PR #70. kind: implementation; stage: implement; blocked-by: [R1].
- [x] R3 Archive and remove only the 58 exact, merged, unchecked-out local refs in [the allowlist](../reviews/reconciliation-2026-10-05.md); recheck expected SHA and worktree exclusion. Preserve dirty/claimed worktrees and every remote ref; remove the independently verified redundant wiki worktree after clean-state/open-file checks, retaining its branch and generated graph archive. kind: implementation; stage: implement; blocked-by: [R1].
- [ ] R4 Verify disposition and review the exact maintenance commit independently. kind: review; stage: review; blocked-by: [R2,R3].
- [ ] R5 Rebase-merge maintenance record under unchanged repository policy. kind: merge; stage: merge; blocked-by: [R4].
- [ ] R6 Verify landed record, remaining PR/claim state, and report remaining ownership/gates. kind: verification; stage: verify-landed; blocked-by: [R5].

Validation: source/tree equivalence, Git ancestry, clean/dirty status, archive readability and hashes, exact-head independent review, GitHub merge readback. No application code changes; no application build required. Shared host one-minute load exceeded 10, so no heavy local builds were started. No new provider capacity or paid model fallback was used.

Operational evidence: CloudWatch log-group discovery succeeded; bounded usage-report retrieval has not produced a complete report. Usage is unknown, not zero. Ajent inbox/diagnose and code-review-graph tools are unavailable in this session; targeted source and Git inspection were used. No deployment, installed-client upgrade, live reliability or acceptance claim is made.

Execution receipt: 58 allowlisted local refs were atomically deleted with expected-SHA guards, then absence verified. All remote refs and all unresolved unique refs remain. The redundant wiki worktree was removed after fresh clean-state and open-file checks; its ignored generated graph files were privately archived with per-file SHA-256 readback, and its branch remains. No other existing worktree was removed. Delivery-stage receipts R4–R6 belong to the maintenance PR linked by this branch; their checkboxes are not advance claims of merge or landing.
