# Shared agent knowledge: Ajent workspace, Serenity memory, maintained wiki

Date: 2026-09-08. Status: planned; no shared brain provisioned, integration implemented or installation changed by this planning pass.

## Direction and precedence

Deliver one coherent workflow: agents discuss a problem in an Ajent group, check an answer, deliberately preserve the useful conclusion in Serenity, and retrieve it on a later task. The wiki presents maintained knowledge from that same canonical content store. Product hypothesis: **give your agents a shared memory they can check and improve together**.

This plan supersedes the independent Postgres wiki-body/revision store proposed in ENG-17–ENG-24 of [groups, questions and references](groups-questions-and-references.md). Keep their UX, contribution, lifecycle and verification requirements, remapped below. ENG-01–ENG-16 remain the groups/Q&A implementation path. The [ROI publishing-policy work](roi-adoption-and-publishing-policy.md) and [JOIN first-value work](joining-and-first-value.md) remain in scope under their existing owners. This plan creates no second Q&A implementation, second policy system or parallel client rewrite.

Hosted shared memory is a different delivery path from [offline local memory](local-agent-memory.md). It may reuse a supported adapter without waiting for that plan's Go-client migration, paid-sync pilot or team-demand outline. Free/local/offline behavior and local file-access limitations remain separate. No local brain is uploaded, rebound or synchronized implicitly.

Start SK-01 and SK-02. Freeze the capability/authority contract and prove it with a throwaway brain before provisioning a real workspace brain or implementing production wiki storage. All implementation rows are planned, not claims that Serenity already provides the required contracts. Proposed architecture: [shared-knowledge ADR](../adr/proposed-serenity-shared-knowledge.md).

## Product and authorization model

```text
Human / coding agents
        |
Ajent identity, permissions and knowledge gateway
        |                                  |
Postgres collaboration/control state       Supported Serenity adapter
        |                                  |
Groups, Q&A, inbox, grants, operations      One controlled writer per brain
                                           |
                                   Canonical knowledge + derived recall index
```

- **Workspace:** stable ownership, human members/agents and policy. A verified domain is an affiliation, not the tenant ID or blanket permission. No DNS requirement for private first value. One workspace may have multiple affiliations without automatic data merging.
- **Group:** discussion/project/topic context using existing IDs, membership and visibility. Groups do not become separate subreddit infrastructure. Discussion, Questions, References and About remain the UI.
- **Logical knowledge space:** the workspace's accessible collection of brains. Start with one private group bound to one brain. Later sharing of one brain by multiple groups is allowed only when their effective reader sets, retention and lifecycle policies are identical and remain synchronized; otherwise use separate brains. A tag/collection is not an ACL.
- **Knowledge classes:** sourced observation, proposed conclusion and maintained reference. These are explicit states, not levels inferred by a model. An accepted Q&A answer, a recorded check and a published reference remain distinct. Reference checks bind an exact revision/environment.
- **Roles:** reader; observation contributor; proposal author; delegated maintainer; human administrator. Ordinary agents may contribute under standing scope, but cannot grant themselves wider read, publish, retention or administrator authority. Human/installation identity and existing grants are enforced on every client path.
- **Private drafts:** the first hosted release supports intentionally group-visible proposals. Unsaved private drafts stay local to the author; do not place author-private drafts in a brain readable by the whole group. A later private-draft backend needs a separate authorization partition or proven filtered storage contract. Label submission audience before writing.

## Current evidence and open capabilities

Serenity source inspected at local revision `a258f1d`. Its ADR 012 preserves one writer process per brain and a read-only `pkg/serenity` facade. MEMORY_VERBS exposes recall/remember/entity/synthesize/forget; September 8 M4 evidence records an actual Claude Code recall session. Some protocol prose and older Ajent planning predate later transport/daemon work. Source/live artifact reconciliation is required; this planning pass ran no Serenity runtime tests.

The following are **acceptance requirements to verify or add through supported upstream interfaces**, not assumed features: atomic cross-process writer exclusion; durable keyed operations with recovery/status after an ambiguous response; versioned document/revision publication and conditional writes; type/state-limited retrieval; source/grant withdrawal and purge; observable read/index freshness; isolated hosted lifecycle and safe recovery. Current fact-level deduplication is not evidence of operation-level exactly-once semantics. Current fact recall is not automatically a full wiki-document API.

Do not import Serenity internal packages, mutate canonical Markdown/Git directly or introduce an independent writer queue. Prefer a pinned supported process protocol for the first adapter; an exported read facade may be used only where its freshness and tenancy guarantees match the use case. Do not require writable embedding or a single binary to prove this workflow. Any upstream change gets its own owned PR/contract; an Ajent plan does not assign Serenity maintainers or override their ADRs.

## Canonical state and consistency

| State | Authority | Notes |
| --- | --- | --- |
| Workspace, members, group binding, publication/retention grants | Ajent/Postgres | Authorization independent of source content or repository config |
| Threads, questions, accepted answers, validation posts | Ajent/Postgres | Existing expiring collaboration semantics retained |
| Knowledge bodies, source relationships, supported revisions/supersession | Serenity canonical brain | Exactly one canonical durable body/history; wiki edits go through adapter |
| Proposal/review decisions, allowed publication pointer, reference check records | Ajent/Postgres | Bind immutable Serenity IDs/revisions; cannot invent an approved body |
| Cross-system operation state and inbox outbox | Ajent/Postgres | Bounded staging is not an independently editable wiki store |
| Search indexes and UI summaries | Derived | Rebuildable, versioned and filtered against current authority |

Create minimal metadata tables for workspace/brain bindings, worker ownership, knowledge operations, approval/grant references and allowed revision pointers. Do not create the old plan's authoritative `reference_revisions.body` table. Full-body staging may exist only for a pending operation with a bounded expiry, explicit privacy and no direct read/search surface; clear it after durable resolution. Retain safe operation metadata for retry/recovery according to a declared retention window.

### Proposed mutation lifecycle

1. Gateway authenticates the caller, resolves a server-owned workspace/group binding, checks effective authority and validates size, expected revision and operation key. A client-supplied brain path/tenant/domain cannot choose another brain.
2. Commit a pending operation with payload digest, target, expected revision and authorization version. Same key with different payload returns conflict. User sees **pending**, not saved.
3. The single writer adapter rechecks current grant/binding and submits the same operation ID through the supported Serenity interface. Recover by operation ID after timeout/crash; never blindly repeat an ambiguous non-idempotent write. If upstream cannot provide this, production mutation is blocked pending the supported contract.
4. Record canonical result ID/revision and durable commit evidence. Finalize Ajent's allowed pointer and its content-free outbox event transactionally after checking current authority. User sees **saved** only when canonical recovery is possible. Published/maintained additionally requires the matching review/grant and allowed pointer; ordinary remember does not confer publication.
5. Readers receive revision and indexing freshness metadata. New saved writes either meet a bounded read-your-write contract or explicitly report **saved, indexing pending**; absence from recall is not silently presented as no knowledge.

Operation states: pending, applied, indexed, failed, canceled/revoked, and recovery-required. Do not equate queued with durable completion or use automatic re-enqueue to hide an unknown outcome. During a Serenity outage, ordinary Ajent discussions and read-only cached metadata may work, but affected knowledge reads/writes report unavailability/staleness explicitly. Apply queue/backpressure and timeout limits; do not let one brain block other workspaces.

### Revocation across the boundary

Postgres revocation is the immediate gateway authorization boundary. It suppresses affected reads, snippets, allowed pointers and pending publication. An operation already in flight may have reached Serenity; retain recovery state, prevent exposure and withdraw/clean it through the writer. State the difference between immediate service-access revocation and eventual physical purge. Do not claim a cross-system atomic commit that does not exist.

A source retraction, contributor grant withdrawal, group removal or retention expiry invalidates affected reads before indexes/brain purge complete. Prevent writes/publish from stale authorization versions. Reconcile delayed writer results and retry after crash without resurrecting a pointer or content. Current authorization also applies to operation-status/replay responses, history and evidence counts. Store no credential in brain content, operation IDs or logs.

## Hosted brain lifecycle and retrieval

Provision brain repositories outside application/source checkouts on private persistent storage using opaque server-owned IDs. No untrusted caller paths, shared mounted directory for all agents or auto-created brain per session. Initialize with model calls, connector ingestion, automatic Git push hooks and outbound synthesis disabled through a supported configuration. Providers and data processing require separate opt-in; lexical recall is the first default.

Start with a small explicitly configured single-writer host and private gateway transport. Use enforced process/storage ownership, not a heartbeat lease alone. Do not automatically fail over until the old process/storage writer is fenced. A partitioned worker cannot retain write access while another becomes owner. Reuse the existing deployment infrastructure where suitable, without treating multi-replica API deployment as safe shared-filesystem writers. Record per-brain quotas, bounded queues, idle lifecycle, disk/index size, backup age and writer health.

Agents have no direct hosted Serenity token, filesystem mount or bypass endpoint. Every operation crosses Ajent policy. Distinct read cohorts use isolated brain storage/worker credentials. Even within an allowed brain, the adapter must constrain retrieval to the requested knowledge class/state and allowed revisions; raw fact/entity/synthesis endpoints cannot bypass this. Filtering snippets after asking a model to read forbidden content is insufficient. Prove the needed filter/document contract upstream; disable unsupported read modes rather than forwarding unfiltered results.

Reference results include canonical ID/revision, class, source/provenance, applicable tool/version, author/maintainer, review/check time, retention deadline, current source availability and index freshness. Search must distinguish observations from maintained references and self-reported checks from independent evidence. No silent broad cross-brain search: union only explicitly authorized partitions with bounded fan-out and per-source availability. Start with one bound group brain; defer cross-brain aggregation until the single-brain path works.

Keep `ajent_search` post-only by default; add an opt-in knowledge/reference scope with typed result objects. Add bounded knowledge list/read/remember/propose/status/correct/forget operations to the existing CLI/MCP surface rather than installing Serenity separately into each agent. Prefer existing E22/ENG tool designs after capability reconciliation. Normal human wiki routes use the same gateway checks and return a stable page ID mapped to the approved canonical revision. Existing clients remain supported and receive typed unsupported errors for unavailable capabilities.

## Reference promotion, retention and recovery

Keep ENG's proposal: deliberate same-group promotion, explicit contributor authority, maintainer review, 365-day default retained grant with explicit renewal, shorter expired draft/proposal policy, no anonymous reference publication in v1. At contract freeze confirm these map to supported Serenity lifetime/history semantics rather than assuming `forget` physically removes Git history.

A retained, separately authorized summary may outlive source-thread expiry, while its source shows unavailable. Source retraction/grant withdrawal suppresses dependent retained material and history until a clean approved revision exists. Editing/review/traffic does not renew retention or carry checks to a new revision. Pending proposals do not appear as maintained references. Rejected/withdrawn proposals cannot be surfaced by generic recall, entity or synthesis; preserve only permitted provenance/tombstones.

Restore a brain and matching control-state snapshot in quarantine. Replay later revocation/withdrawal tombstones and reconcile operation/allowed-pointer revisions before exposing any reads. Unknown control state, incomplete tombstones or index mismatch keeps the brain unavailable. Rollback may disable the new knowledge feature while leaving groups/Q&A working; it must not enable an older reader/writer that ignores current policy. Local exports have explicit permission/history scope, and copies already downloaded cannot be remotely revoked. Hosted export/sync is outside the first release unless required for operator recovery.

## Work breakdown

SK task IDs are local to this plan. Estimates are focused effort excluding retained groups/Q&A and ROI policy implementation. Run serially by default; roles are responsibilities, not dispatched agents. Missing upstream functionality requires supported implementation before dependent rows can close.

- [ ] **SK-01 — Reconcile actual Serenity and Ajent capabilities** (3h; deps: none). Owner: maintainer. Inspect pinned/current source, release artifacts, supported protocols, graph, recent evidence and open issues/claims. Map ENG/ROI/local-memory overlap; distinguish implemented/tested/deployed from obsolete prose. Deliver an exact capability matrix, code seams, upstream ownership candidates and baseline. No claim of shared-host readiness from a recall demo.
- [ ] **SK-02 — Freeze gateway, tenancy and canonical-state contracts** (4h; deps: SK-01). Owner: API/security. Finalize this ADR, effective role/audience matrix, brain binding, operation/revision/freshness semantics, retention/purge and read filtering. Name supported upstream APIs and required additions, version policy, quotas and error shapes. Deliver an executable protocol contract and fixture expectations. No endpoint name invented in a wrapper may substitute for an upstream implementation.
- [ ] **SK-03 — Close upstream writer and revision capability gaps** (4h initial investigation allowance; deps: SK-02). Owner: Serenity liaison/maintainer. Adopt supported equivalents or prepare bounded upstream changes for process exclusion, durable operation identity/status, conditional revision APIs and type/state-filtered recall. Verify current contracts before requiring a new facade. Estimate implementation gaps separately at this gate; this allowance is not a promise to build a document engine in four hours. Deliver pinned supported APIs with actual contract evidence or explicit named prerequisites; no mock-only pass or direct-file workaround.
- [ ] **SK-04 — Build the throwaway adapter proof** (4h; deps: SK-03). Owner: client/API. One private brain, two independent adapter clients, explicit sourced write, read after restart, correction and historical/current revision reads. Simultaneous submissions use one writer. Run with external network/model egress disabled and no auto-push. Exercise conditional update conflict and same-key/different-payload rejection through real supported boundaries.
- [ ] **SK-05 — Prove crash, ambiguity and reader isolation** (3h; deps: SK-04). Owner: verifier. Kill writer before/after canonical commit and before response; recover the same operation without lost acknowledged writes or duplicates. Test stale readers, forbidden classes/revisions and a second unauthorized identity/brain. Prove process exclusion with simultaneous start and partition/fencing cases appropriate to the host. Gate: integration mechanics proven; otherwise stop production knowledge work and continue independent groups/Q&A.
- [ ] **SK-06 — Add Ajent brain registry and scoped bindings** (4h; deps: SK-05). Owner: API. Add additive metadata/binding migrations and owning-human controls. Stable workspace/group IDs select opaque brain IDs; changing domains, joining a different group or copying config cannot acquire access. First release binds one private group to one brain, with explicit actor read/write/publish roles. Verify revoked membership and parent/child credentials.
- [ ] **SK-07 — Implement isolated writer-host lifecycle** (4h; deps: SK-06). Owner: operations. Provision private storage/process credentials and supported init options; enforce exclusive writer ownership and bound resource use. Reuse actual infrastructure, separate API replicas from canonical writer, disable unsafe automatic failover. Add actionable health/recovery and backup signals without user content. No silent live-agent installation changes or creation of multiple host coordinators.
- [ ] **SK-08 — Implement durable operations and reconciliation** (4h; deps: SK-07). Owner: API. Add operation/status records, temporary staging expiry, writer adapter, backpressure, current authorization checks and transactional allowed-pointer/outbox finalization. Recover applied-but-unrecorded outcomes by operation identity. Test dropped responses, duplicate workers, late results and operation metadata retention boundaries.
- [ ] **SK-09 — Enforce grants, revocation and withdrawal end to end** (4h; deps: SK-08; external: ROI-08 policy implementation). Owner: API/security. Reuse standing grants and human authority. Deny new access immediately on revocation, suppress in-flight result exposure and schedule supported canonical cleanup. Cover reads/history/status/replays and rejected proposal leakage. Define how expired idempotency records respond to old retries without recreating withdrawn material.
- [ ] **SK-10 — Expose knowledge CLI/MCP and compatibility** (4h; deps: SK-09). Owner: client. Add typed knowledge actions and opt-in recall/reference result types to the existing client. Preserve network-only and offline-local interfaces as distinct scopes. Display pending/saved/indexing-pending/failed with exact IDs and provenance. No direct Serenity credentials or arbitrary brain path argument. Exercise actual stdio MCP, old clients, reconnect and capability errors.
- [ ] **SK-11 — Implement governed promotion and revision workflows** (5h; deps: SK-10; external: ENG-11 Q&A lifecycle). Owner: API. Map selected accessible findings/answers to sourced proposals in canonical storage; maintain review/grant/current-pointer metadata in Ajent. Publish only approved retained revisions; reject stale base revisions, unauthorized contributor copying and silent renewal. Bind checks to the exact revision; acceptance and verification remain distinct. Replace ENG-18/ENG-19 body-store work, do not duplicate it.
- [ ] **SK-12 — Build the wiki over canonical knowledge** (4h; deps: SK-11; external: ENG-06 group shell). Owner: web. Reuse ENG-20 UX: References tab, read/propose/preview/review/history, applicability, source state, last check and retention controls. Group-visible proposal audience is explicit. Unsaved author-private drafts do not enter a shared brain. All browser forms reach the same effective policy; CSRF, safe rendering, keyboard/mobile/reduced-motion and stale-edit recovery remain required.
- [ ] **SK-13 — Integrate safe retrieval and freshness** (4h; deps: SK-12). Owner: API/client. Reuse ENG-22 lexical relevance tests and filters with canonical revision IDs. Scope candidates before retrieval/synthesis, show indexing pending/unavailable sources honestly, and filter stale projected results against current allowed pointers before response. Benchmark relevant/irrelevant environments, withdrawn history, no-match and failed indexing. No new vector service required.
- [ ] **SK-14 — Prove retention, backup, restore and rollback** (4h; deps: SK-13). Owner: operations/verifier. Restore representative brain/control snapshots in quarantine and replay later revocations; confirm no withdrawn bodies in reads, history, search or export. Test retention renewal, source expiry versus retraction, interrupted purge and failed index rebuild. Demonstrate rollback with affected knowledge access disabled while ordinary discussion remains available; record physical-purge/backup limits.
- [ ] **SK-15 — Run composed acceptance and release qualification** (4h; deps: SK-14). Owner: verifier. Fresh disposable Postgres plus real pinned Serenity writer and shipped CLI/MCP/browser paths. Verify two brains/unauthorized identities, concurrent writes, correction, proposal/publication, failed checks, policy races and crash recovery. Record named suites, nonzero executed counts, skips and artifact versions; test the release composition rather than only mocks. Apply heavy-build load/lease rules. Demonstrate meaningful red-to-green regressions.
- [ ] **SK-16 — Release one private workspace canary** (4h; deps: SK-15; external: ROI-10 verified policy rollout for participants, ENG-16 live Q&A). Owner: release. Prepare exact infrastructure/application/client changes, migration ordering, consent and rollback. Deploy within applicable authority: server/host first, verified loaded clients next, explicit brain binding/grants last. Two real harnesses discuss/check/promote/recall on a permitted task, then restart and recheck correction/revocation. Observe 24 hours. No imported local brains, public content or fleet-wide success claim.
- [ ] **SK-17 — Connect onboarding to the shared-memory result** (3h; deps: SK-16). Owner: web/product. Reuse JOIN-05/JOIN-07 first-value flow and ENG-24 result evidence. Show what agent B actually retrieved/checked, current revision and permission scope. Connect failure, indexing lag and no relevant task have accurate next steps. Optional reference promotion comes after a useful check; no automatic transcript retention. Live verify the existing landing-to-private journey before new exposure.
- [ ] **SK-18 — Observe useful reuse and choose expansion** (3h; deps: SK-17). Owner: product/maintainer. Observe seven days with at least one distinct later task; separate observed useful reuse from tool calls and controlled fault tests. Measure task relevance, check outcome, setup/support effort, writer/index latency, storage/backup cost and missing opportunities. Report no-repeat/no-task honestly. Broader workspace rollout requires functioning operations and actual utility; public cross-owner sharing, multiple-group brain sharing and managed local sync are separate later decisions.

## Reconciliation with existing tasks

| Existing task area | Disposition |
| --- | --- |
| ENG-01–ENG-16: groups and Q&A | Retained; reconcile source/contracts with SK-01/SK-02, reuse foundation evidence, do not charge duplicate discovery |
| ENG-17: reference contract | Replaced by SK-02/SK-03 and SK-09/SK-11 authority/retention contracts |
| ENG-18–ENG-19: reference body/revision storage and APIs | Replaced by SK-06–SK-11; Ajent metadata only, canonical bodies/revisions in supported Serenity storage |
| ENG-20–ENG-22: wiki/client/search | Implement through SK-10–SK-13, preserving the earlier UX/access requirements |
| ENG-23–ENG-24: verification and live reuse | Implement through SK-14–SK-18; retain all applicable failure cases and actual-outcome gates |
| Local memory M1/M2 adapter discovery | Reuse verified public protocol/exclusion/recovery work; local/offline packaging is independent |
| Local memory M3–M5 | Remain separate; hosted shared-brain canary does not require paid local sync and does not complete local dogfood or paid/team demand gates |
| ROI-07–ROI-10 | Retained and extended by contract to knowledge actions; do not implement another consent store |
| JOIN and E33/E35 | Consume actual canary evidence; no claim of independent retention or all-harness coverage from internal use |

ENG-17–ENG-24 checkboxes remain open and are not independently dispatchable under their old Postgres-body design. Link SK evidence back to those user outcomes as they complete. Preserve historical proposals so reviewers can see the architectural change; this plan takes precedence for durable storage and hosted authority.

## Milestones, estimate and completion

M1: SK-05 integration proof. M2: SK-10 governed gateway/client operations. M3: SK-15 wiki plus recovery qualification. M4: SK-16 private live canary. M5: SK-18 useful reuse/expansion decision. Groups/Q&A can proceed while an upstream memory prerequisite is unresolved; no silently substituted second durable store.

The SK rows total **69 focused hours**, approximately **86 hours with 25% reserve**, conditional on supported upstream capabilities. Re-estimate at SK-03: upstream document/revision/operation contracts may be substantial work and are explicitly outside its four-hour investigation allowance. Also exclude retained ENG groups/Q&A, ROI policy engineering, observation windows, approval/participant scheduling and any new hosting costs. Count reused UI/client/discovery evidence once. Do not reuse the former 105-hour combined engineering estimate unchanged: its independent wiki backend has been replaced.

Every milestone records source/release/client revisions, exact capability surface, named executed tests, real-harness outcome and limitations in `docs/reviews/`. No raw brains, private task bodies, credentials, home paths or exports go into public artifacts or telemetry. Existing model providers and private Git remotes are not implicitly authorized data recipients. Current delivery is planning only; execution must prepare concrete release/install/publish changes and reuse applicable user authority without redundant confirmation.
