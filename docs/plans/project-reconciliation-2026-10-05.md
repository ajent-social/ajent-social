# Project reconciliation — 2026-10-05

Scope: reconcile in-progress service work, retire superseded candidates, preserve pending planned work and active ownership. Production deployment, new roadmap implementation, account settings and destructive changes to the original checkout are excluded.

## Evidence and disposition

Base: `1853cf7f975e728865ff25115d88c34f0a89842b` on origin/main. The original checkout at `da23d51` is 253 commits behind and three ahead; it is not the current product. September 24 reconciliation already preserved 439 working files and recorded why replaying the old source would regress current work.

- PR #68 (knowledge and private-worker fixes), #75 (compatible UI polish and maintenance records), and #76 (AMSL wiki) are merged. Their old checklist/branch state is not evidence of unfinished delivery.
- PR #70, head `f69e5b7962c5757410aeafca2cc3f2121351da9f`, is superseded. Its analytics implementation is tree-identical to main; the complete tree differs only because it lacks the subsequently added DNS browser test and its documentation. Independent Luna audit confirmed this. Closed October 5; do not merge the stale candidate.
- PR #80, head `bdaea6a832cc327c601a95c1c58a21d778b7d4d8`, remains active planned Go-installer/Python-client retirement. Remote claim `R-crew-python-service-retirement` is owned by another coordinator; preserve its dirty worktree. Infrastructure, UI and container jobs passed; its test job in [run 37339761974](https://github.com/ajent-social/ajent-social/actions/runs/37339761974) failed cloning the client repository with “repository not found”. This is checkout/access evidence, not an application test result. Its signed-off source, independent review, merge and landed checks remain that lane's responsibility.
- Discovery work has existing P02D/S03 claim refs. Preserve those worktrees and pending branch content; this maintenance task does not take ownership of another agent's work.
- E21–E27 growth/activation/cohort work, E30–E35 operations/fleet work, and E37 membership contain pending plan rows. Preserve them. Unchecked rows are not proof that source is absent or authority to initiate deployment/outreach. Future execution needs task-level reconciliation with current code and evidence.
- Original dirty checkout, local reports, infrastructure configuration, and unresolved unique branch patches are retained. Age, divergence and archive existence alone do not justify deleting authored work.

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
