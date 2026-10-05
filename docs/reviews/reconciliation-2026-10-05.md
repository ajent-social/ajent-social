# Local ref reconciliation — 2026-10-05

Base: `1853cf7f975e728865ff25115d88c34f0a89842b`. This is a local cleanup allowlist, not permission to remove remote refs, active worktrees or unmerged content.

Every listed head is an ancestor of this main revision and none is checked out in any registered worktree. Current remote claims cover the Python retirement and discovery lanes, which are excluded. Before removal, an all-ref bundle was created in a private recovery directory on the external SSD and `git bundle verify` passed. The recovery directory also retains the exact ref inventory and candidate JSON. Removal rechecked worktree membership, ancestry and the expected ref SHA and used a single compare-and-swap ref transaction. All 58 deleted refs were then verified absent; no expected head changed.

## Exact local ref allowlist

| Branch | Preserved head |
| --- | --- |
| `chore/archive-superseded-work` | `e11402662413b345df6d14681d222d552185041e` |
| `docs/ajent-launch-film-treatment` | `f958fe8ac956f54659777a824954887649cb045b` |
| `docs/apex-cutover` | `f7edd955b45e1d30d8d8ba0afdeb7bfb5232e8f3` |
| `docs/e30-workspace-membership` | `52232b06e691e74ad6372eec34c47aed6df83818` |
| `docs/improvement-release-evidence` | `7cd10bc91b68d805787d57b38f54f475de561c2c` |
| `docs/moltbook-e21-t21.1` | `1c72b9e38c2daf082508bcb89cc749ead1bf2dd1` |
| `feat/agent-integration` | `e70028f7ab2a136e7821ec4fbe1bbd00b506a0b9` |
| `feat/automatic-joining` | `5bd8e30d88381bf784011cf88d9ecb6efc5b3c60` |
| `feat/help-improve` | `a40c4ca4d75e4a4cc81370d21132f800db8a8f16` |
| `feat/help-improve-runtime` | `d974440bce0d6d0c37a5c9cd19759238d71cce38` |
| `feat/improvement-triage-engine` | `348f60f37c95651673fd413bf4cc66352108a597` |
| `feat/openrouter-chat` | `37fdea43abdfb630f743129aad13f70b556b4f6f` |
| `feat/public-conversion` | `695f47eb58c623be9bfa4af1425685ac90757d98` |
| `feat/public-experience` | `195e432b1c5a3d51a2efcbe03066a51982e7ef9a` |
| `feat/reliability-loop` | `21e1f1fc5870bc5fb38746c8fd99415b7e806781` |
| `feat/verified-participation` | `5122075a52c44897b728bb61474fc2f5582edf6e` |
| `fix/account-design-system` | `fe5fe2b52f511f23816b62edac2f2f582388221a` |
| `fix/account-journeys` | `20a48c526e49a46f3db30959b76f9e3d453812da` |
| `fix/account-theme-contrast` | `632c625434aa62d081fb210403f3bb75fa42bcd5` |
| `fix/action-recovery` | `40de0f9e092507c1e0c38c68134c162ba52f30c7` |
| `fix/agent-token-budget` | `65f8588a0c33e0a80d53781a0912b3d93e4bde7b` |
| `fix/apex-routing` | `fd1e13f19e06060c7da8528a7a2dae34c20eea04` |
| `fix/chat-form-clearance` | `21ebc7a63a5b8091a867f603c64c20c8ff001cbd` |
| `fix/chat-keyboard-scroll` | `dc8b4e03fd93c6af9387b643c80424674c250ed1` |
| `fix/ci-image-revision` | `89aed9aae8e330a2f20302e301e8156c77c4d0d2` |
| `fix/content-read-recovery` | `b2b32b1388681235d6d941892acc960ffebf46bf` |
| `fix/direct-analytics-review` | `d4bdd4dcaca1af74424ea8505fb8fddc44758fb7` |
| `fix/feed-navigation` | `38c18fffc02589eca3b731365c45fe4bcba55664` |
| `fix/form-validation-recovery` | `a61cef3d48bac1624183e45450804581be07d8b9` |
| `fix/help-accessibility` | `59db603959e8908401dcefd581a834c3eb613e7c` |
| `fix/mobile-usability` | `c09e7cf5c1bb9a69e6b59b2235a021fea440bce4` |
| `fix/overnight-token-audit` | `3c4df70eb73a579c7477941b76bc6fca3986f28f` |
| `fix/owner-error-pages` | `77362df956bc64f5bc2399fc24de01eed3369109` |
| `fix/private-outbox-fairness` | `99b25bc59cf5f58c80bc330df3a7b14e7dc4e8a8` |
| `fix/public-account-navigation` | `7a9e732a7a7fa396d6be1cef847ddef566f6bbe4` |
| `fix/reconcile-live-main` | `47f2df5e9bd959b7a4dd4d03260be469ec4da5bd` |
| `fix/reconcile-ui-polish` | `3f30f4ca1f5ab90a6c0c5cec3f4034df8429fbd4` |
| `fix/redact-analytics-paths` | `8fdd57e6d8e5b364c4cf05fcac00a5d69524b09a` |
| `fix/remaining-ui-states` | `64e1ec631fa2990f9809b6680f2124c8daea34c0` |
| `fix/remove-chat-notice` | `7523e05292ab7f63315e0426c6d61536d978ff7a` |
| `fix/restore-glass-chat` | `2721210bfef87354d186327a51f948bc5732ba52` |
| `fix/save-navigation` | `2a9d8d28ed56817fb59caacaa166ccf5a952bdf7` |
| `fix/session-drafts` | `5548f53ea55f5c745d85053e244a8db10b844c71` |
| `fix/session-navigation` | `e0e9bedc0b98adf8fb4b29bfc8a2d220601dda0c` |
| `fix/shared-page-width` | `3ec9682d927fe22a712af16ff937a1e772815a47` |
| `fix/shared-skill-symlink` | `a4d3b82a22c5a3dd0811b4a7ae8bec2b33046ea6` |
| `fix/workspace-dashboard` | `063eeb4f22ef14c7bc6b7ee33466fbf19e94fc99` |
| `release/discoverability-20260907` | `ff30e6effdb44d57c1c7c8463fc24c8e0b36d6e5` |
| `review/all-pages-20260907` | `f6480ed0006d9b1b29e29e6010e36d4fb65b5f4e` |
| `review/pr14-navigation` | `f5de809b3935a963a6889a6aa100dc960a324f7a` |
| `review/pr25-consulting` | `0b8287f9ea60b3de0247d59e06ac8041804428ef` |
| `review/pr33-marketing` | `be0fd08d85992983ce64e5a474b21dc13efceeb3` |
| `review/pr51-film` | `2f2f57bb386e8fa96fc93c207c418410b1b660fc` |
| `review/pr54-fetch` | `6de73b64e07dbfd18abc0aef35e70ff879e64764` |
| `review/pr55-membership` | `90d994058b7f22f626b494476833c9c16ab68f57` |
| `review/pr9-cutover` | `bb65abe23e61c212bef57fc17449c047a0792fe1` |
| `test/ui-audit-coverage` | `cd32ef3e5d415e9cecdbca99f379dd31bf0ebdb5` |
| `test/ui-dns-browser` | `e7abc4f405fde85b10fce39ed88cbee28432a927` |

## Protected work

Preserve the dirty original checkout, all dirty/claimed worktrees, all branches with unresolved unique content, main, the active maintenance branch and all remote branch refs. Detached worktrees with infrastructure config/binaries are outside this allowlist. No broad clean, reset, path removal or primary-checkout rewrite is authorized by this record.

## Operational and CI evidence

PR #80's current infrastructure, UI and container jobs succeeded; the test job in [run 37339761974](https://github.com/ajent-social/ajent-social/actions/runs/37339761974) failed during the client checkout: Git reported that the client repository was not found. This does not establish an application test failure or success. The current retirement owner must resolve access and complete its independent exact-head review, merge and landed checks.

## Redundant wiki worktree

Independent review found the wiki implementation and plan at `cf8c1c1f69d86e67ef74a74df81c617ea8dac4e3` already represented in main; worker differences are later DNS changes on main. Fresh status showed no tracked or untracked authored changes, and open-file inspection found no active use. The only ignored files were generated code-graph metadata; they were privately archived and verified by per-file SHA-256 readback. The redundant worktree was removed without force, retaining its local branch and bundle recovery. All other existing worktrees remain.
