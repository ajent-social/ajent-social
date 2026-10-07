# Retirement delivery unblock

Owner instruction: access granted; fix and merge PR80 and PR81. Source base is main1853cf7; retirement candidate begins at bdaea6a. Work happens in isolated SSD worktrees. Preserve the prior dirty retirement worktree, its claim, all unrelated changes and the original checkout. No production deployment or release publication is included.

- [x] U1 Configure least-privilege cross-repository CI checkout and preserve pinned client revision. Owner: coordinator. kind: implementation; stage: implement.
- [x] U2 Separate isolated DNS browser verification from the ordinary race suite without weakening zero-skips. Owner: inventory_review. kind: implementation; stage: implement.
- [x] U3 Reconcile vulnerable infrastructure dependencies to compatible patched versions. Owner: dependency_fix. kind: implementation; stage: implement.
- [ ] U4 Verify integration on existing CI runners and appropriate local static checks. Owner: coordinator. kind: verification; stage: verify; blocked-by: [U1,U2,U3,U8,U9].
- [ ] U5 Independently review full PR80 at exact final head; fix accepted findings and refresh verification. Owner: maintenance_review. kind: review; stage: review; blocked-by: [U4].
- [ ] U6 Guarded rebase merge PR80 and verify landed source/checks. Owner: coordinator. kind: merge; stage: merge; blocked-by: [U5].
- [ ] U7 Refresh PR81 on landed main, verify, review and rebase merge its maintenance record. Owner: coordinator. kind: verification; stage: verify-landed; blocked-by: [U6].

Acceptance: pinned private client checkout succeeds with read-only repository access; actual Go enrollment/link test executes; ordinary suite and dedicated isolated DNS browser test execute with no failed/skipped selected tests; infrastructure vulnerability scan passes; full independent exact-head review has no unresolved blocking findings; both PRs read back merged and their landed revisions verified. Delivery receipts belong in the PRs; checkboxes do not pre-claim future merges.

The graph tools are unavailable in this session, so source inspection uses Git and targeted reads. Shared Mac load exceeds10; heavy verification uses existing compatible self-hosted runners and no new paid capacity. Secrets never enter Git or logs. GitHub rejected a read-only deploy key because client repository policy disables deploy keys. Preserve that policy. CI checkout requires the owner-provided AJENT_CLIENT_READ_TOKEN Actions secret, scoped to Contents: read on the client repository; the current broad personal credential is not transferred to CI. No deploy key was registered and the generated ephemeral private key was deleted.

Accepted review follow-up U8: gate new-install entry points until release trust is commissioned; fail closed before any downloaded PowerShell executable runs. Existing installed Go clients can continue setup. This is a source/merge task; public release and production activation remain separate. lane: agent.

U9 supplies adversarial installer fixtures, including valid signatures, wrong keys, tampered manifests and missing-verifier failures. Owner: dependency_fix. kind: implementation; stage: implement; lane: agent; blocked-by: [U8].

Implemented U8 and U9: release readiness gates and signature verification on both platforms are covered by actual Unix and PowerShell adversarial fixtures. At the October 6 handoff, U1 workflow changes were complete but the owner-provided secret was absent. U4-U7 remained pending. The continuation below supersedes that access blocker.


## DGX continuation — 2026-10-07

Owner supplied the read-only Actions secret. Run37426828612 attempt2 fetched and
built the exact pinned client, then exposed a race in the real approval fixture:
it sets Server.Origin after httptest starts serving. Earlier access failures had
prevented this test from executing. Preserve the race and zero-skips gates.

- [x] U10 Initialize approval-fixture origin before serving. Owner: DGX Luna worker; kind: implementation; stage: implement; blocked-by: [U1].
- [ ] U11 Verify U10 with the actual pinned client under race detection, then refresh full CI. Owner: coordinator; kind: verification; stage: verify; blocked-by: [U10].
- [ ] U12 Independent exact-head review of the race fix and integration records. Owner: independent DGX Luna reviewer; kind: review; stage: review; blocked-by: [U11].

U6 now additionally depends on U12. Active coding/review work has moved to an
isolated DGX task workspace, with separate worktrees and caches; clean task heads,
plans and receipts were transferred and bundle hashes matched. Mac copies and the
legacy dirty checkout remain preserved. Existing Linux CI runners supply full
verification. No Mac builds, tests, containers or coding workers are started.
