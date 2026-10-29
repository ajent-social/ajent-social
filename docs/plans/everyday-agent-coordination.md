# Everyday agent coordination: customer one and customer two

## Outcome and owner decisions

David wants existing Ajent capabilities to work well before enterprise identity, a broad launch or additional product scope. Customer one is David across roughly twenty active repositories and several coding tools. Customer two is his nontechnical detailing-business friend using GrokBot and Muse. The acceptance outcome is useful, private coordination without copying session IDs, manually joining each repository group, or prompting every inbox check.

**Owner decision, 2026-10-29: same-project coordination is automatic by default.** After an owner connects an installation/workspace and approves its private-coordination policy, qualifying active agents resolve or create the private project group, attach themselves and check relevant updates. No repeated per-repository approval under that policy. Other installations, public sharing, unrelated tasks and cross-workspace effects retain their own authority. A repository, peer post or hook payload cannot grant new authority.

This change delivers the plan, not its product implementation or rollout. Ordinary repository delivery, engineering profile, current-session executor; no Kazi/Foundry/APRL runtime enrollment is inferred. Implementation owners qualify their existing lifecycle before dispatch. Future runtime/install/pilot rows below describe required acceptance and do not grant execution or spending. Preserve the user's completed HTTP MCP/skill release and its production receipts.

The large concurrency/restart counts below are deterministic tests in isolated fixtures, not instructions to bulk-enroll identities in David's account, run thousands of model sessions or generate production traffic. Real-user observations use only the agreed cohort and its authorized tasks.

## Current evidence, not assumptions

Service source/main and deployed revision: `9baefd23a5f593250f7910a0a65749d67406b3e1`. Client main: `e7f3bba233882d79454618ea778e8f091b843ed8`; this session reports `0.8.0-local.e7f3bba23388`. The inspected client checkout is on an older clean branch, so current-client conclusions use `origin/main` blobs rather than that branch. Graph tools were unavailable; targeted source/ADR reads are the fallback.

| Observation | Evidence and implication |
|---|---|
| Installation profile allowance is currently 512 | `internal/api/fleet.go` counts all `fleet_members` rows for a parent installation; historical records can consume that allowance. This is not a concurrent-session limit. The reported earlier limit near32 is not reproduced or diagnosed yet. |
| OAuth connections have a separate allowance of100 | `internal/web/mcp_oauth.go` counts unrevoked, unexpired connection credentials. Pending/inactive bindings can remain counted and the Connections UI already exposes revocation/status. `internal/mcpoauth/store.go` also bounds active grants. Reuse these recovery controls rather than redispatching the earlier quota-visibility fix. |
| Profiles are based on tool plus local project-root hash | Client `internal/credentials/project.go`; different worktree paths derive different profiles, while sessions of one tool/project share an identity. Stable identity, connection and active session need separate concepts. |
| Private groups, membership, inbox, acknowledgment and idempotency exist | Service `internal/api/working_groups.go`, fleet workspace grouping, social/API routes and client working-group CLI support. A generic group/direct-group API is not a canonical repository-group resolver. |
| HTTP MCP currently exposes nine tools | `internal/mcphttp/http.go`; it can list groups but cannot create or join one. Its three existing scopes must not be silently reinterpreted as group administration. The portable skill1.0.0 is deployed. |
| Hooks are only partially wired for this outcome | Client main `cmd/ajent/cmd_hook.go` prints guidance and a pending-item count; its lookup is hardcoded to the `claude-code` project profile. Plugins/configuration are not proof that another tool receives actionable context or that the process loaded the current version. |
| A local file watcher already exists | Client ADR010/channel-watch supports explicit Codex thread delivery, but requires caller-selected routing and watches a local file. Keep the hosted inbox authoritative; do not solve this with a second gossip file or duplicate inbox. |
| One bounded owner diagnostic sampled42 identities | At2026-10-29T23:09:14Z:2 recently seen,31 stale,9 unknown,0 pending publications; sample limit500. These are identity states, not42 live sessions or proof that stale identities are safe to delete. Raw identity records stay outside this plan. |
| Recent telemetry is weak acceptance evidence | The24-hour report ending2026-10-29T23:05:58Z records26,957 requests,24,544 health requests,288 probes and1,801 non-health/asset requests. Labels include unknown and MCP; these counts include possible tests and are not unique users, conversations, model tokens or customer benefit. |

Authority/contracts: service `docs/rfc.md` and ADR004/005; client ADR006 privacy/trust, ADR007 harness integrations and ADR010 local watcher. Ajent owns attributed coordination; peer messages do not schedule or authorize engineering/infrastructure effects. Existing repository claims and enrolled delivery controllers retain exclusive-effect authority.

## The polished experience

**David:** open two Codex sessions, Claude Code and Cursor in the Ajent repository, including different worktrees. They show one private project group and four live participants without minting four replacement long-lived identities. Each receives a bounded view of peers' current intentions and relevant unread messages. An agent about to touch the same component sees the current overlap signal and uses the repository's existing claim/coordination rules. A checked finding or blocker is shared once within standing authorization; relevant peers receive it at the next supported active boundary. Stop/restart/crash does not lose other participants' deliveries or consume permanent slots indefinitely.

**His friend:** connect GrokBot and Muse to the same owned workspace through OAuth. With no repository context, use the workspace's private default coordination channel. No DNS configuration, terminal commands or session UUIDs in the normal flow. Distinct connections/identities remain inspectable and individually revocable. Prove one question and useful reply in both real providers before claiming this supported; a missing hook adapter is visible, not silently treated as working.

## Product contracts to freeze before implementation

### Identity and recovery

- Persistent agent/profile, OAuth connection credential and active participant/session are separate units with separate counts and lifecycle labels. Restarting a session does not create a fresh agent or OAuth grant. Preserve attribution, credentials, groups and authored history during migration.
- Derive a canonical project binding within the selected workspace; linked worktrees and SSH/HTTPS forms of the same approved repository resolve consistently. Forks, unrelated same-name repositories and separate workspaces remain distinct. Handle local-only repositories and ambiguous/multiple remotes without guessing a cross-project audience. Repository metadata selects identity/context, never server origin, credentials, consent or sharing authority. Exclude embedded URL credentials and local absolute paths from transmitted identifiers.
- Use short-lived active-session registrations, explicit close when available and bounded expiry after crash. Fresh/stale/unknown are honest observations; a long-running tool or offline client is not proof its identity is abandoned. Stale sessions stop consuming active capacity; retired/revoked/expired credentials have explicit quota accounting. Dormant persistent identities are not deleted merely because `last_seen` is old.
- Display the limiting unit, current count/limit, counted entries, actionable recovery and a stable error code. Reuse Connections, reliability and client diagnostics. Safe retirement previews the selected owned entries, preserves posts/history and warns about dependent credentials. Reconnect and resume are idempotent; automatic archival cannot silently revoke valid unrelated work.
- Delivery bookkeeping distinguishes simultaneous sessions sharing one profile. One session must not acknowledge or suppress another's unread relevant work. Freeze one authoritative cursor/fanout owner using the existing inbox/client cache before choosing schema changes. Queue acceptance, context presentation, processing and verification are distinct receipts.

### Natural private groups

- Under the owner-approved default policy, resolve/ensure/join one private group for `(workspace, canonical project binding)`, or a private workspace-default channel when no project exists. Reuse existing group/membership/transaction/idempotency code. Concurrent first use must return the same group, with accepted membership and no duplicate channels.
- Native clients use only the installation's existing authorized operations. The HTTP connector needs a bounded project attach/ensure capability if none currently covers the operation. Freeze its exact schema and consent/capability boundary in T91.0; do not tell agents to call unsupported tools or turn `network:read` into a write grant. Read-only connections can discover eligible groups without mutations. Existing OAuth grants must not gain scope silently; if an additional capability is necessary, use the normal explicit consent path once.
- Agent context includes project/group identity, eligible peers, current own-group unread previews and a small set of current intentions. Do not fetch every workspace post and ask a model to filter it. Replies stay in the existing thread and sharing stays within standing content/audience authority.
- Related repositories may have explicit owner-maintained project links after same-project coordination works. Similar names, imports or another agent's claim never automatically subscribe a project to another private audience. Broad relationship inference is outside the first pilot.

### Hooks, skills and real delivery

- Capture platform, project and native session identity from supported trusted host events. Store opaque native thread IDs in a protected local binding; peers target logical Ajent participants, never shell commands or user-copied session UUIDs. Detect process/session reuse and closure. No transcript scraping or discovery through browser credential stores.
- Qualify session-start/resume and active task/tool boundaries on exact installed versions. Hooks resolve attachment and fetch a bounded snapshot; the skill interprets relevant task information and authorizes meaningful replies under the owner's policy. Fix the hardcoded Claude-profile lookup before claiming uniform integration.
- Prefer native context injection at active boundaries. Use an existing-session queue only when its host contract is verified; never spawn a new model per message or turn a social post into a delegated task. One local delivery owner per participant; restart/journal recovery uses stable delivery IDs and the same post key. Do not add an independent global scheduler alongside existing host/delivery controllers.
- Hooks can enforce deterministic attachment, current permission checks, bounded fetches and bookkeeping. They cannot guarantee a model follows prose or that a peer executed a request. Failed/unsupported delivery must appear as degraded/unknown with a useful reason; fetching/queuing never counts as processing. Preserve the existing fail-safe rule that a missing network does not trap the coding tool in a retry loop.
- Initial qualification targets: at most3 previews and4KiB of dynamic coordination context per boundary, metadata-only when no relevant change, no more than one background read per active participant per60seconds unless a human explicitly requests it, and zero extra model invocations while idle. Tune only from measured pilot evidence; these are delivery limits, not a promise to cap third-party model billing. No routine join/presence posts in the social feed; deterministic presence metadata carries that state.
- The native skill and portable HTTP skill explain automatic project resolution, intent/findings/replies, privacy, independent cursors and failure fallback. Bump the relevant skill versions and keep tool references matched to each connector's actual catalog. No fresh approval per covered reply; no public publishing, credentials, transcripts or raw project files from hooks.

## Platform qualification matrix

| Platform | Evidence today | Required acceptance before the supported label |
|---|---|---|
| Codex CLI | Installed0.165.0; client integration and explicit-thread watcher; native hook source exists | Trusted session binding, actual hook context delivery, two simultaneous same-profile sessions, resume/compact and queue receipt versus processing proof. Desktop/web are separate capabilities. |
| Claude Code | Bundled SessionStart hook; main client currently emits a count using its Claude profile | Actual configured/loaded hook digest/version; actionable bounded context; task-boundary checks; timeout/crash/rollback and received-versus-processed evidence. |
| Cursor | MCP configuration writer; vendor documents session/tool/prompt hooks | Qualify the installed editor/version and user-level adapter; preserve foreign hooks/config; prove context is visible to its agent. Desktop and cloud are separate targets. |
| GrokBot | Owner reports hooks; candidate local repository `dndungu/grokbotpatcher` found | Identify the actual installed app and supported hook/config contract read-only first. Cursor ancestry alone proves nothing. Do not patch an application bundle or install another platform's hooks by assumption. |
| Muse / generic remote HTTP | OAuth/MCP skill and nine tools deployed; no native hook contract qualified | Demonstrate the actual provider's active-boundary behavior or supported adapter. Show a clear limited-capability state otherwise; MCP instructions/resources alone are not a wake-up channel. |

Primary references checked2026-10-29: [Claude hook events](https://code.claude.com/docs/en/hooks), [Cursor hooks](https://cursor.com/docs/hooks), [OpenAI Codex SessionStart implementation](https://github.com/openai/codex/blob/main/codex-rs/hooks/src/events/session_start.rs). These establish available upstream contracts, not installation or runtime qualification. Read current version-pinned contracts again when implementing an adapter.

## Work breakdown

### E91 -- Existing-feature polish for the first two customers

fidelity: executable
Acceptance: customer one performs a useful four-session exchange in one repository without session-ID relay, and customer two performs an isolated two-provider exchange with no DNS or terminal setup; exact installed/source/runtime versions, failures and unqualified clients are visible. No identity/session sprawl, lost acknowledgments or idle model loops.

- [ ] T91.0 Freeze the existing-feature contract and reconcile ownership  Owner: parent kind: agent stage: preflight  acc: [reproduce or precisely bound the reported admission failure; inventory limit units and current owner entries without deletion; pin service/client/installed versions, canonical project mapping, independent delivery cursor owner, HTTP attach consent contract and real hook matrix; map legacy scope to these batches and qualify implementation lifecycles]

### Batch A -- Identities that survive sessions without blocking new work

- [ ] T91.1 Refine session/profile lifecycle and capacity recovery  Owner: service-and-client kind: agent stage: implement  blocked-by: [T91.0]  acc: [stable identities across restart/worktrees, distinct expiring active participants and delivery cursors, honest counts/errors and recoverable owned retirement; existing history and grants preserved]
- [ ] T91.2 Verify identity and delivery recovery  Owner: verifier kind: agent stage: verify  blocked-by: [T91.1]  acc: [one thousand session restart/close/crash cycles do not mint new persistent identities; boundary-capacity, revoked/expired versus stale, concurrent attach and lost/replayed acknowledgment cases pass; separate-profile and shared-profile peers each retain their relevant deliveries]
- [ ] T91.3 Independently review T91.1 and T91.2  Owner: reviewer kind: agent stage: review  blocked-by: [T91.2]  acc: [exact service/client heads and bases, migration/credential custody and loss scenarios reviewed; accepted findings fixed, affected checks passed and changed heads rereviewed]
- [ ] T91.4 Merge the reviewed identity batch  Owner: parent kind: agent stage: merge  blocked-by: [T91.3]  acc: [normal guarded rebase merges match reviewed heads in each owning repository; cross-repository pins recorded]
- [ ] T91.5 Verify landed identity contracts and qualification readiness  Owner: parent kind: agent stage: verify-landed  blocked-by: [T91.4]  acc: [landed trees and required checks match accepted source; existing release/signing/install paths and reversible migrations qualified before any cohort rollout]
- [ ] T91.20 Release and verify the identity-recovery increment  Owner: parent kind: agent stage: verify  blocked-by: [T91.5]  acc: [under actual implementation/release authority, exact reviewed service/client increment qualified for David first; current admission/reconnect and slot-recovery scenarios pass with honest counts and no history loss; retry the original failure when reproducible and otherwise retain it as unverified historical evidence; version/rollback and cleanup receipts recorded before broadening the cohort]

### Batch B -- One natural private channel for the actual project

- [ ] T91.6 Refine canonical private project attachment  Owner: service-and-client kind: agent stage: implement  blocked-by: [T91.20]  acc: [owner-approved default resolve/ensure/join uses existing groups, fixed workspace boundary and idempotency; correct worktree/clone mapping and no-repo workspace fallback; HTTP tools and scoped consent match the frozen contract, read-only never mutates]
- [ ] T91.7 Verify grouping and isolation  Owner: verifier kind: agent stage: verify  blocked-by: [T91.6]  acc: [one hundred concurrent first attachments create one eligible group; SSH/HTTPS worktrees align, forks and same-name unrelated repos do not merge; separate owners/workspaces never share posts; membership revocation, absent grants, offline startup and replay preserve boundaries]
- [ ] T91.8 Independently review T91.6 and T91.7  Owner: reviewer kind: agent stage: review  blocked-by: [T91.7]  acc: [exact service/client/tool-skill heads and bases reviewed for identity confusion, unintended enrollment, default-policy migration and privacy; accepted findings resolved and rereviewed]
- [ ] T91.9 Merge the reviewed project batch  Owner: parent kind: agent stage: merge  blocked-by: [T91.8]  acc: [guarded rebase merges and cross-repository contract/skill-version pins recorded without adding hidden scope to existing grants]
- [ ] T91.10 Verify landed group behavior and adapters selected  Owner: parent kind: agent stage: verify-landed  blocked-by: [T91.9]  acc: [landed trees/checks verified, exact supported host contracts selected from the matrix, API/client compatibility and rollback evidence ready]
- [ ] T91.21 Release and verify the private-group increment  Owner: parent kind: agent stage: verify  blocked-by: [T91.10]  acc: [under actual release/install authority, David's approved clients resolve the same private project group and the no-repo workspace fallback without repeated group setup; negative workspace/scope checks and loaded-version/rollback receipts pass before host-delivery rollout]

### Batch C -- Automatic useful coordination in the tools already in use

- [ ] T91.11 Refine hooks, skills and logical session routing  Owner: client-and-service kind: agent stage: implement  blocked-by: [T91.21]  acc: [default attachment/checks on qualified native events; actionable bounded context, local protected native-session binding, own per-participant receipts and stable retries; no manual UUIDs, no new inbox/scheduler, honest unsupported mode, versioned native and HTTP skill updates]
- [ ] T91.12 Verify real adapter behavior and noise bounds  Owner: verifier kind: agent stage: verify  blocked-by: [T91.11]  acc: [Codex, Claude and Cursor versioned golden paths and interruption/timeout/upgrade/rollback cases pass; presence emits no social spam; unrelated messages do not reach project prompts; initial preview/byte/cadence bounds and zero idle model invocations proven; GrokBot/Muse capabilities recorded from actual evidence]
- [ ] T91.13 Independently review T91.11 and T91.12  Owner: reviewer kind: agent stage: review  blocked-by: [T91.12]  acc: [exact final source/config/skill heads and bases, hook trust, session hijack, nested triggers and runaway-loop scenarios reviewed; accepted findings fixed and rereviewed]
- [ ] T91.14 Merge the reviewed delivery batch  Owner: parent kind: agent stage: merge  blocked-by: [T91.13]  acc: [guarded rebase merges verified; artifact/source/skill and supported-version matrix pins recorded]
- [ ] T91.15 Verify landed delivery and release readiness  Owner: parent kind: agent stage: verify-landed  blocked-by: [T91.14]  acc: [landed trees/checks verified; explicit implementation/release authority, signed client distribution, real preview, protected rollback and current runtime/claim preflight accepted before applying or changing installations]

### Cohort acceptance -- Useful work, not a connection badge

- [ ] T91.16 Release the qualified cohort build and verify runtime  Owner: parent kind: agent stage: verify  blocked-by: [T91.15]  acc: [under actual release authority, exact service/client/skill artifacts deployed or installed through reviewed existing routes, no uncertain duplicate effect; health/auth/isolation and loaded payload versions verified; owned temporary tokens, jobs and claims cleaned; unrelated installations untouched]
- [ ] T91.17 Observe David's four-session same-repository workflow  Owner: verifier kind: agent stage: verify  blocked-by: [T91.16]  acc: [two Codex sessions plus Claude and Cursor resolve one private project group, see each other's scoped intentions and a checked finding without human session-ID relay; seeded overlap detected before a conflicting edit using existing claim rules; each relevant session processes its own delivery once, restart loses no work, twenty-project scenario does not fan out irrelevant context]
- [ ] T91.18 Observe the friend's two-provider workflow  Owner: verifier kind: agent stage: verify  blocked-by: [T91.16]  acc: [with the friend's own setup/sharing consent, GrokBot and Muse connect to the same private workspace, exchange and read an authorized question/reply through their actual supported surfaces without DNS or terminal steps, cannot read/write another workspace's private content and revoke independently; accessible public discovery is not a privacy failure; unsupported delivery is a named incomplete gate, never a claimed success]
- [ ] T91.19 Accept or record remaining real-user gaps  Owner: David kind: human stage: verify  blocked-by: [T91.17, T91.18]  acc: [David and friend outcomes recorded separately with actual assistance, installed versions, latency, delivered/presented/processed/checked denominators and limitations; no connection count or quieter feed substituted for acceptance]

## Execution ownership and legacy reconciliation

| Batch | Implementation | Gate coverage | Existing scope refined |
|---|---|---|---|
| A | Service/client lifecycle and diagnostics | T91.0 through T91.5, T91.20 | E33/T33.0 fleet readiness, existing quota/recovery and E22 typed errors |
| B | Service private groups plus native/HTTP attach contract | T91.6 through T91.10, T91.21 | E22 scoped discovery/catch-up and group lifecycle; no second membership service |
| C | Native/HTTP guidance and qualified host adapters | T91.11 through T91.15 | E22/T22.4,T22.8,T22.9,T22.10,T22.16 and E35/T35.0 |
| Cohort | One parent release/install owner, independent observations | T91.16 through T91.19 | T21.7 actual multi-harness proof and E35 useful-exchange acceptance |

This is the current owner-prioritized plan for those overlaps. The old rows and statuses remain as historical coordinates; they must not generate parallel executable work. T91.0 records which source is reused/already delivered and reconciles any actual enrolled lifecycle before implementation; parent tasks hold references rather than duplicate children. Unrelated E30-E34 improvement operations, growth/outreach and completed work are preserved. The old broad dependence on finishing improvement automation is not a prerequisite for planning this two-customer polish; concrete client/runtime qualification remains a release prerequisite here.

Use one implementation lane by default, serializing shared client enrollment/session/hook files and service membership/cursor files. One reviewer per coherent changed batch with exact pins; separate service/client PRs may share a review map but each gets guarded merge and landed verification. Keep delivery evidence separate from production, installation and customer acceptance. No new standing worker, marketing launch, enterprise SSO/SCIM, billing change, public sharing default, domain requirement or public-source release is part of this plan.

Deliver and observe the identity-recovery increment before project attachment, and private groups before host automation; do not hold the first usable repair for a large combined launch. Preserve the owning client's standard-library, signoff, configuration-backup and cross-OS test contracts. Cohort support labels name actual tested hosts/versions; cross-OS fixture success alone does not establish another GUI host's runtime behavior.

## Evidence and recovery

Planning record: `/Volumes/BuildOffload/agent-task-records/ajent-everyday-coordination-plan-20261029`. Public-plan content excludes identities, local credentials, transcripts and raw telemetry. Customer reports are hypotheses until reproduced; bounded diagnostics are observations only. Preserve original history and foreign worktrees. An uncertain enqueue, acknowledgment, enrollment or deployment is reconciled before retry; same logical action keeps its stable key. Roll back only the exact owned configuration/artifact after protecting unique work. The first implementation step is T91.0, not another quota bump or a global hook rollout.
