# Ajent local agent memory and optional paid sync

## Context

Date: 2026 09 07 UTC. Work types: engineering and product strategy. Status: implementation plan, no implementation or deployment performed by this planning pass. Decision: [ADR 001](../adr/001-local-agent-memory.md).

Deliver a Go aj client that gives coding agents durable, inspectable local Serenity memory. One installation serves many sessions. Local memory is free and usable offline. Ajent remains the network for deliberately shared findings. Paid managed cross-machine sync is a later hypothesis to validate, not a prerequisite for local usefulness.

Do not alter the active apex cutover or Pulumi migration. Do not treat the historical production plan or old launch plan as a reliable inventory of current code. Rebase implementation work onto current main and reconcile in-flight client/domain work before each task.

## Discovery summary

- Existing areas: cmd/aj/main.go is a small Go network CLI; internal/web/onboarding/join.py owns richer self-service enrollment, local profiles and scopes; ajent_mcp.py exposes network tools; ajent_integrations.py merges user-level harness configuration; plugin SessionStart currently prints guidance only. Port the installed client's behavior, not just the old Go CLI.
- Serenity pkg/serenity exposes Open, Recall and CheckPlan over a brain directory. It exports no writer. ADR 012 requires one writer per brain. Markdown/fences/shards are canonical; SQLite is not a replacement for those files. Recall can call models and record spend when configured, so use an explicit offline lexical path for the default experience.
- Earlier inspection found successful CI at Serenity ced6411. During planning, main advanced to 13883a0 with T4.1 daemon core merged; executable daemon wiring, MCP and MEMORY_VERBS remained incomplete. The new daemon PID guard reads then writes a file without atomic exclusion (internal/server/daemon.go); simultaneous start safety is not established by its sequential second-instance test. Verify current task status before adopting or assigning upstream work. A green component suite is not proof of integrated Ajent memory.
- Both knowledge graphs were stale or lacked matching nodes; source and public package contracts were read instead. No new build or runtime validation occurred for this plan.
- Business evidence: David has a dozen or more sessions and wants continuity. There is no measured retention uplift, willingness-to-pay evidence, chosen price or established sync margin yet.

## Use case summary

These 12 scoped use cases are PLANNED, not claims of current wiring. P0: 8; P1: 2; P2: 2. Existing network capabilities remain compatibility requirements. Durable catalog: this table; merged machine catalog: .claude/scratch/usecases-manifest.json.

| ID | Priority | Actor and observable outcome |
| --- | --- | --- |
| UC-M01 | P0 | Human installs/upgrades aj without losing existing identity or harness settings |
| UC-M02 | P0 | Agent remembers an explicit sourced fact while offline |
| UC-M03 | P0 | Another authorized local session recalls the fact after service restart |
| UC-M04 | P0 | Human selects project/personal memory and inspects its local path |
| UC-M05 | P0 | Agent corrects a fact with an inspectable supersession chain |
| UC-M06 | P0 | Human forgets a fact and understands history/purge limits |
| UC-M07 | P0 | Twelve sessions write without lost acknowledged operations or duplicate retries |
| UC-M08 | P0 | Coding agent uses memory through MCP without uploading local content |
| UC-M09 | P1 | Human authorizes sharing one selected finding through existing Ajent permissions |
| UC-M10 | P1 | Human exports/restores a brain and continues without Ajent service access |
| UC-M11 | P2 | Paying user synchronizes chosen brains across devices with conflict/recovery controls |
| UC-M12 | P2 | Team shares governed memory without leaking private or unrelated project data |

## Scope and deliverables

| ID | Deliverable | Owning role | Acceptance |
| --- | --- | --- | --- |
| D-M1 | Compatibility and upstream host contracts | Ajent client + Serenity maintainers | Fixture matrix and supported write lifecycle agreed in code/API review |
| D-M2 | Offline local memory preview | Ajent client + Serenity maintainers | Restart, contention, isolation and corruption drills pass |
| D-M3 | Installable agent integration | Ajent client/release | Real harnesses and existing credentials work from released artifacts |
| D-M4 | Local dogfood evidence | QA/product | Seven-day measured use across at least twelve sessions |
| D-M5 | Paid-sync decision | Product/security | Demand, conflict design and unit economics justify a pilot or explicitly defer it |

Out of scope initially: replacing Ajent Postgres; copying all transcripts; email/file connectors; autonomous consolidation/model spend; cloud memory hosting; automatic publication; paid billing; native Windows service support; team ACLs. Initial release platforms: macOS arm64/amd64 and Linux arm64/amd64; WSL can use Linux artifacts. Native Windows needs a later lifecycle/keychain/locking plan.

## Intended interface and boundaries

Proposed commands, to freeze in M1.1: aj install, aj doctor, aj mcp, aj memory init/status/path/remember/recall/inspect/correct/forget/export, and aj service start/status/stop. Preserve existing network commands or provide documented compatibility aliases. Command names are planned, not presently available.

Store brains at the OS user data directory under ajent/brains/<brain-id>; keep credentials/config in their existing supported location. Private directories use 0700 and files 0600 where applicable. A project gets a stable locally authorized binding, not an identity inferred from a domain, remote URL or untrusted checked-in file. Default to project memory in a bound project; personal memory requires an explicit binding. Two Git worktrees share a brain only through an approved binding. Sessions are actors, not automatically new brains.

MCP exposes distinct ajent_memory_remember/recall/inspect/correct/forget tools. ajent_search remains network search. Responses carry brain ID, memory ID, source reference, actor/session attribution, timestamps and supersession state. Remember accepts explicit content plus provenance and an idempotency key; it does not invent sources. A repeated key with a different payload fails. Raw observations are distinguishable from verified claims. Tool schemas validate input sizes and unknown fields before execution.

One per-user local service routes to one writer per open brain. Use a user-private Unix socket with peer/permission checks and bounded requests. MCP invocations do not instantiate independent writers. A root lock excludes standalone Serenity writers too; if upstream lacks such exclusion, add it upstream before concurrent use. A service acknowledgement means durable recovery is possible, not merely that work entered a RAM queue. Queue overload returns a bounded retryable error.

Default operation does not contact Ajent, model providers or a Git remote. Disable initialization's automatic post-commit push hook in the embedded profile through a supported upstream option. MCP scope limits prevent accidental cross-project tool access, but same-user agents with unrestricted filesystem access can read user files; do not market that as a security sandbox.

## Checkable work breakdown

### M1 -- Go compatibility and upstream memory seam
fidelity: executable

Acceptance: a supported upstream writer host and an offline command test prove a durable write/read cycle without regressing existing enrollment. Estimates below are focused 30-90 minute implementation slices, not delivery promises; discoveries that exceed a slice must be split before dispatch.

- [ ] M1.1 Freeze client and memory contracts. Owner: client. Est: 60m. lane: agent. deps: []. verifies: [UC-M01, UC-M08]. acc: [versioned fixtures enumerate every installed Python command and MCP tool with success/error shapes and proposed memory schemas]. Area: docs/contracts/local-memory.md, internal/client/testdata/. Test subtask: derive fixtures from join.py, ajent_mcp.py, integrations and latest merged scope behavior; enumerate intentional compatibility breaks, JSON output, exits, origin redirects and credential handling. No guessed API routes.
- [ ] M1.2 Package aj for testable dispatch. Owner: client. Est: 60m. deps: [M1.1]. verifies: [UC-M01]. acc: [aj command fixture tests run through the executable dispatcher with injected HTTP and filesystem dependencies]. Area: cmd/aj, internal/client. Test subtask: existing CLI success/error transcripts stay compatible; stdout contains only command output, never credentials or diagnostics.
- [ ] M1.3 Migrate credentials atomically. Owner: client. Est: 90m. deps: [M1.2]. verifies: [UC-M01]. acc: [Python credential/profile fixtures load in Go without reminting identities and failed migration leaves original bytes usable]. Area: internal/client/credentials. Test subtask: symlinks, permissions, malformed files, account switching and parallel migration; preserve rollback copy without printing tokens.
- [ ] M1.4 Port enrollment and scope reconciliation. Owner: client. Est: 90m. deps: [M1.3]. verifies: [UC-M01, UC-M09]. acc: [real HTTP fixture-server enrollment and rerun preserve identity; untrusted project domain grants are rejected]. Area: internal/client/enrollment and scopes. Test subtask: current self-service retry/pending behavior, domain/private mode, revocation and legacy-origin transition; assert status and response bodies at the real handler boundary. Never revive the old setup-token registration path.
- [ ] M1.5 Port network operations and browser login. Owner: client. Est: 90m. deps: [M1.2]. verifies: [UC-M01, UC-M09]. acc: [Go client passes the frozen network operation fixture matrix including post retries and short-lived browser login]. Area: internal/client/network. Test subtask: search/read/feed/groups/agents/events/post/me, pagination/resync, 401/403/429, timeout and redirect refusal; reuse idempotency keys on retries.
- [ ] M1.6 Specify the upstream writer host. Owner: Serenity liaison. Est: 60m. lane: agent. deps: []. delivers: [Serenity RFC/ADR amendment and current E4 dependency mapping]. Area: docs/contracts/serenity-host.md and upstream review. Name lifecycle, options, capabilities, durable acknowledgements, offline initialization, process locking and schema versions. Resolve with Serenity maintainers; this Ajent plan does not silently supersede Serenity ADR 012. No modification of upstream plans by this row.
- [ ] M1.7 Expose supported writer lifecycle upstream. Owner: Serenity. Est: 90m. blocked: Upstream acceptance of M1.6 and required E4 writer/verb work. deps: [M1.6]. verifies: [UC-M02, UC-M07]. acc: [an external Go consumer opens one brain writer and a second process cannot acquire that brain until the first exits]. Area: Serenity public host package and writer lifecycle, exact paths fixed by M1.6. Test subtask: external-package test, real subprocess exclusion, close/reopen and crash lock recovery; no internal imports or duplicate queues. Adopt already-merged equivalent work instead of duplicating it.
- [ ] M1.8 Expose deterministic offline operations upstream. Owner: Serenity. Est: 90m. deps: [M1.7]. verifies: [UC-M02, UC-M03, UC-M05, UC-M06]. acc: [public host remembers and lexically recalls a sourced observation after restart with outbound network denied]. Area: supported Serenity host and existing memory engine. Test subtask: pin schema, source provenance, same-key replay, correction and forget semantics; initialization installs no automatic push hook, models stay disabled. Missing existing engine behavior becomes a separate upstream prerequisite rather than a stub.
- [ ] M1.9 Add Ajent memory adapter. Owner: client. Est: 60m. deps: [M1.2, M1.8]. verifies: [UC-M02, UC-M03]. acc: [Ajent adapter writes and recalls through a pinned supported Serenity release with no direct canonical file mutations]. Area: internal/memory and go.mod/go.sum. Test subtask: external package compatibility and error mapping; record Git/keychain prerequisites and actionable failures. No private-module build credentials in released artifacts.

### M2 -- Safe local memory preview
fidelity: executable

Acceptance: twelve local sessions share a durable brain with scoped access, offline behavior, clear deletion semantics and recovery evidence. This is reachable when M1's seam lands; blocked rows must not be dispatched early.

- [ ] M2.1 Implement brain registry and bindings. Owner: client. Est: 90m. deps: [M1.2]. verifies: [UC-M04, UC-M08]. acc: [project and personal brain selection is stable across subdirectories and unauthorized repository bindings cannot select another brain]. Area: internal/memory/registry. Test subtask: moved paths, explicit worktree reuse, path traversal, symlinks, unset binding, account changes; local scope is independent of network domain verification.
- [ ] M2.2 Implement local service ownership and IPC. Owner: client. Est: 90m. deps: [M1.9, M2.1]. verifies: [UC-M07, UC-M08]. acc: [two simultaneous service starts yield exactly one socket owner and twelve clients use its single brain writer]. Area: internal/localservice, cmd/aj. Test subtask: real socket requests assert success/error envelopes, stale socket recovery, peer permissions, cancellation and oversized requests; no TCP listener by default.
- [ ] M2.3 Implement durable mutation and backpressure contract. Owner: client. Est: 90m. deps: [M2.2]. verifies: [UC-M02, UC-M07]. acc: [all acknowledged mutations survive forced service exit and identical retries create one logical operation]. Area: internal/localservice and supported Serenity adapter. Test subtask: crash before/after commit and before reply, differing-payload same key, queue saturation, disk-full error. Let Serenity own durable operation records; do not add an inconsistent second memory log.
- [ ] M2.4 Wire memory CLI operations. Owner: client. Est: 90m. deps: [M2.3]. verifies: [UC-M02, UC-M03, UC-M04, UC-M05]. acc: [separate aj processes remember, inspect, correct and recall a sourced fact after service restart]. Area: cmd/aj memory dispatcher. Test subtask: executable JSON/text outputs, stdin content, missing source, offline lexical recall, empty results and bounded result size; no fake citations.
- [ ] M2.5 Implement forget and export contracts. Owner: client. Est: 90m. deps: [M2.4]. verifies: [UC-M06, UC-M10]. acc: [forgotten content stays absent from default recall after restart and index rebuild; clean export restore preserves remaining sources and supersession chains]. Area: internal/memory/export and CLI. Test subtask: repeated forget is idempotent and reimport cannot silently resurrect a tombstoned source; historical Git content explicitly remains recoverable unless a supported purge is used; exclude credentials, socket files and derived caches from export; rebuild index on restore. Document backup/history and already-published-copy caveats and refuse to call logical forget secure erasure.
- [ ] M2.6 Detect external edits and index recovery. Owner: Serenity + client. Est: 90m. deps: [M2.4]. verifies: [UC-M03, UC-M04, UC-M07]. acc: [an out-of-band Markdown change is surfaced for reconciliation and cannot silently overwrite a concurrent tool update]. Area: existing Serenity reconciliation and Ajent doctor adapter. Test subtask: corrupt index rebuild, malformed page isolation, dirty Git tree, interrupted writes and Git unavailable. Serialize repair with the same writer; preserve human edits and report stale reads.
- [ ] M2.7 Port network MCP and add local memory tools. Owner: client. Est: 90m. deps: [M1.5, M2.5]. verifies: [UC-M08, UC-M09]. acc: [a real stdio MCP transcript invokes local remember/recall offline and existing network tools retain their contracts]. Area: internal/mcp, cmd/aj mcp. Test subtask: initialize/capability negotiation, notifications, malformed/deep/oversized payloads, cancellation, error responses and stdout purity; publish remains a separate explicit operation and never sends the brain automatically.
- [ ] M2.8 Prove scopes, concurrency and no automatic egress. Owner: QA. Est: 90m. deps: [M2.6, M2.7]. verifies: [UC-M03, UC-M07, UC-M08]. acc: [twelve subprocess sessions complete 120 unique writes plus retries with zero lost acknowledgements, duplicates or cross-scope tool results and zero outbound calls in offline mode]. Area: tests/localmemory. Test subtask: deny outbound traffic, seed two project brains, test forged scope/tool arguments and instruction-like memory content, restart mid-run; report exact executed counts and failures. Same-user filesystem access is outside tool-isolation claims.
- [ ] M2.9 Run formatting, lint and CI release qualification. Owner: release. Est: 60m. deps: [M1.4, M2.8]. verifies: [infrastructure]. acc: [Go formatting, vet, race tests, vulnerability checks and platform builds pass with named local-memory suites executed]. Area: existing CI and release configuration. Test subtask: preserve existing server and Python compatibility suites; CI carries builds, no heavy shared-mini compile. These are preview binaries only until M3 production verification.

### M3 -- Installer, harness rollout and useful local dogfood
fidelity: outline

Intent: release one Go binary with user-service lifecycle and idempotent integration migration; validate currently supported Claude Code, Codex, Cursor, Gemini CLI and Copilot adapters against official contracts at implementation time. Do not claim popularity rankings or support from config-file generation alone. Hooks supply bounded local context and guidance; no transcript capture or model calls. Preserve Python rollback until live parity is proven.

Exit: released/checksummed artifacts work on the supported platform matrix; rerun/uninstall preserves unrelated config and user memory; fresh and upgraded installs work against the current production origin; real harness sessions invoke tools. Seven days across at least twelve sessions and two projects produce at least 20 inspected useful recalls, zero lost acknowledged writes, zero unintended uploads, and documented latency/maintenance cost. Count helpful recalled decisions separately from raw tool calls. These are proposed acceptance thresholds, not observed metrics.

- [ ] M3.0 PLAN: expand install, hooks, migration and dogfood tasks. Owner: client/release/product. Est: 90m. kind: plan. deps: [M2.9]. delivers: [executable M3 tasks, real-harness matrix, rollout/rollback and seven-day evidence protocol]. Include browser golden/error tests if /join changes; production installer and human short-lived login verification; supported release signatures/checksums, model opt-in/budget disclosure, Git/keychain dependency UX, local-only use without signup, and session-start timeout behavior.

### M4 -- Paid managed sync validation and design
fidelity: outline

Intent: sell managed continuity and recovery, while preserving free local access/export and DIY Git. First measure multi-machine demand from actual memory users. Do not choose a price from intuition or charge for unfinished synchronization. Record at least five multi-machine user discussions and three explicit paid-pilot commitments at a stated candidate price before authorizing implementation; David's own use is dogfood, not external demand evidence.

Exit: demand gate plus a cost model covering stored bytes/history, transfer, requests, backups, support and recovery; a security-reviewed protocol specifies device identity/revocation, encryption/key recovery, offline operation IDs, conflicts, tombstones, retention, billing grace and entitlement boundaries. Replay/convergence and restored-device resurrection tests are specified. Raw Git push/pull into live brains is not accepted as a concurrent sync protocol. No free-local lockout on cancellation.

- [ ] M4.0 PLAN: expand the paid-sync discovery and technical pilot. Owner: product/security. Est: 90m. kind: plan. deps: [M3.0]. blocked: M3 implemented and seven-day dogfood exit evidenced. delivers: [demand report, cost model, security ADR and executable pilot plan or explicit defer decision]. Recheck current provider/pricing documentation then. Keep hosted pilot implementation gated on measured demand and protocol review; use existing AWS/Pulumi only if the design requires infrastructure.

### M5 -- Shared team memory
fidelity: outline

Intent: separate personal memory from team-governed project knowledge with explicit sharing, membership/revocation, provenance, retention and audit. Local unrestricted files cannot enforce organization revocation after download; product policy must state that limitation.

Exit: organizations demonstrate demand and an approved threat model plus end-to-end authorization tests cover joins, departures, shared corrections, exports and private-to-team publication. Pricing and administration follow that evidence.

- [ ] M5.0 PLAN: expand team-memory scope. Owner: product/security. Est: 60m. kind: plan. deps: [M4.0]. blocked: Managed-sync pilot exit and team demand evidenced. delivers: [team use cases, policy/ACL design and executable implementation plan]. Do not advertise availability from this outline.

## Parallel work and waves

Capacity: up to two implementation agents; this plan does not launch them. Each wave has one agent per listed task, with merge/rebase checks between waves. Upstream tasks require maintainer ownership, not assumed availability.

| Wave | Tasks | Agents |
| --- | --- | --- |
| 1 | M1.1, M1.6 | 2 |
| 2 | M1.2, M1.7 (only after external gate) | 2 |
| 3 | M1.3, M1.8 | 2 |
| 4 | M1.4, M1.5 | 2 |
| 5 | M1.9, M2.1 | 2 |
| 6 | M2.2 | 1 |
| 7 | M2.3 | 1 |
| 8 | M2.4 | 1 |
| 9 | M2.5, M2.6 | 2 |
| 10 | M2.7 | 1 |
| 11 | M2.8 | 1 |
| 12 | M2.9 | 1 |
| 13 | M3.0 | 1 |

Waves are a safe schedule, not extra dependency edges. If upstream is blocked, M1.3-M1.5 and M2.1 can proceed without it. Serial durability and recovery gates are intentional. M4.0 and M5.0 are scheduled only after their external evidence gates; completion of a planning row is not completion of its phase.

## Timeline and milestones

| Milestone | Gate | Planning estimate |
| --- | --- | --- |
| Contract ready | M1.1, M1.6 | First working day |
| Writable embedding proven | M1.7-M1.9 | Upstream-dependent; no committed date |
| Local preview | M2.9 | Approximately 3-5 engineering days after host capability exists, including integration contingency |
| Released local dogfood | M3 implementation exit | At least seven elapsed days of observation after installation qualification |
| Paid-sync go/defer | M4 evidence gate | Demand-driven, no promised ship date |

## Risk register

| Risk | Impact / likelihood | Mitigation |
| --- | --- | --- |
| Unsupported writable embedding | High / current gap | Explicit upstream contract and pinned release; no internal imports |
| Concurrent tools/direct edits corrupt history | High / likely without controls | Process lock, one writer, crash receipts, reconciliation tests |
| Local content leaks via models/hooks/Git/network | High / plausible | No default egress, disabled push hooks, separate publish operation, deny-network tests |
| Python-to-Go loses authority/scopes | High / plausible | Frozen fixture matrix, real API tests, atomic migration and rollback |
| Recall repeats stale or malicious claims | High / plausible | Provenance, supersession, bounded context, untrusted-data framing; no automatic execution |
| Git history makes forget misleading | High / inherent | Separate logical deletion and historical purge; explicit backup caveats |
| Sync resurrects deleted data or leaks tenants | High / future | Tombstones/convergence/key design and adversarial tests before paid pilot |
| Local dependency or model costs surprise users | Medium / plausible | doctor, offline lexical default, explicit model opt-in and cost controls |

## Operating procedure and definition of done

Use isolated worktrees and claim resources before shared-plan rewrites. Do not deploy or change DNS as part of memory implementation. Go builds run in CI under existing project constraints. Recheck origin/main and upstream status before dispatch; the shared checkout has unrelated in-flight changes. Preserve existing task ownership in other plans.

Every implementation row includes tests; service/MCP/network contracts require real boundary requests and response assertions. Break the relevant behavior to demonstrate a genuine-red test before declaring it protective. Run gofmt, vet, scoped tests, race and required security checks in CI. No UI exists in M1/M2, so browser tests are not applicable there; M3 adds them if onboarding UI changes. Record executed suites/counts, skips, revisions and actual limitations.

Merge and release through existing repository procedures. M1/M2 code acceptance is internal preview only: user-facing delivery remains incomplete until M3 deploys the installer/documentation to production and verifies fresh and upgraded real installations against the live Ajent service, including domain permissions and browser login. Local memory must also pass with that service unavailable. Do not remove Python support or advertise sync before the respective gates.

## Hand off notes

Start with M1.1 and M1.6. Read Serenity pkg/serenity/serenity.go, ADR 012 and E4 plan at current main. Upstream E4 T4.1/T4.5 are relevant dependency candidates, not blanket prerequisites for every local read. Coordinate the host change with their owners. This plan creates no Serenity assignment and certifies no unattended task contract.

## Progress log

2026 09 07: Added M1-M5, 12 planned use cases and ADR 001; executable depth is limited to Go compatibility and safe local preview; install/dogfood, paid sync and team memory remain gated planning phases.
