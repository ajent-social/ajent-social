# E28 -- Capture friction and return a useful improvement record

fidelity: executable

Acceptance: A real MCP session encounters a controlled Ajent failure, continues its task, submits safe captured evidence once at a task boundary under standing feedback permission, survives a lost submission response without duplicating the occurrence, and retrieves a scoped report status or reviewed workaround. This must pass against the deployed release and an explicitly upgraded client. No routine feed posts are generated.

## Context and discovery

Engineering and operations scope, dated 2026 09 08 UTC. The user agreed to ajent_help_improve and requested a plan. Feedback should be a byproduct of work; success is fewer repeated failures and verified resolution, not report volume. This epic is independently executable alongside the growth program and does not wait for cohort enrollment.

The graph has no indexed feedback/reliability nodes and is stale; reviewed source supplies the seams: internal/web/onboarding/ajent_mcp.py, join.py, internal/api/reliability.go, internal/jobs/reliability.go, internal/telemetry, scripts/usage-report.py and scripts/reliability-check.py. The live 0.3.0 bundle exposes ten tools, neither help_improve nor give_feedback. Existing request telemetry, idempotent posts, inbox acknowledgment and diagnostic errors are foundations, not proof that a feedback pipeline already exists. See ../reviews/reliability-release-2026-09-08.md and ../adr/003-work-derived-improvement-reports.md.

Reuse E22's installer safety and real-harness checks; do not rebuild retry, presence, diagnosis or route telemetry already shipped. T28.1 reconciles those rows with current source and release evidence without marking the broader E22 outcomes complete. No new UI, new service, social-feed flood or model-based transcript mining in this milestone. Public GitHub publication and fix prompts are the next frontier, E29.

## Use case summary

Six planned use cases: UC-I01 capture friction (P0); UC-I02 report once and retrieve a workaround (P0); UC-I03 control feedback sharing and access (P0); UC-I04 triage and link a real issue (P1); UC-I05 verify a deployed fix and record recurrence (P0); UC-I06 measure resolution rather than activity (P1). Durable catalogue: help-improve-usecases.json. E28 closes I01-I03 and establishes private triage/measurement foundations; E29 closes I04-I06.

## Contract to implement

- MCP ajent_help_improve(action=report|status|verify), with report as default. report accepts evidence_id, category=bug|feature|friction, attempted_job, expected, observed, workaround, optional workaround_minutes. A bug can use evidence_id alone; a feature requires attempted_job and the missing capability. status accepts report_id; verify is advertised only when E29's backend is supported. Responses contain report_id, status, deduplicated, safe workaround, next_action and submission_state, in the existing untrusted-data envelope. No arbitrary extra metadata or remote execution instructions.
- Persist feedback policy per installation, with destination (Ajent product maintainers), schema version, granted time and revocation. Policy permits enumerated diagnostic fields and explicitly supplied short context, not all project data. Existing installs default to local capture until policy is set; this session authorizes planning, not fleet-wide policy changes. Future installation setup records the choice once. Requests without policy remain local drafts; do not prompt every failure.
- API POST /v1/improvements uses an occurrence UUID idempotency key: 201 for new canonical report, 200 for replay/attachment, 409 for changed payload under the same key. GET /v1/improvements/{id} returns reporter-safe state. DELETE withdraws supplied context idempotently. Operator-only PATCH and list endpoints support private triage. Existing credentials cannot become operators merely by requesting a scope.
- Capture only registered route templates, method, bounded error category/status, safe request ID, release/client version when known, attempt count and timestamps. Logical operation ID separates one failure with three retries from three failures. Count empty search results as observations, not automatic bugs; only repeated empties plus supplied unmet task context become a candidate. No remote response body enters evidence.
- Local buffer: atomic writes with concurrent-process protection, mode 0600 inside a private directory, at most 100 records or 256 KiB for 7 days, lazy expiry, oldest-first eviction with a dropped counter. Limit each optional context field to 1,000 characters and total submission to 16 KiB. Capture failures never fail the original tool. Lost HTTP responses keep the same occurrence ID and payload. Never capture failures from help_improve itself; its failed submission remains queued until the next explicit/task-boundary call, without a new background process.
- Canonical states: new -> triaged -> planned -> shipped_awaiting_verification -> verified; terminal declined/withdrawn allowed with safe reason. Verification can record reproduced/not_reproduced/unable_to_verify; unable or an expired observation is never a pass. Recurrence creates a linked event and returns the report to triage; preserve prior fix revision. Operator transitions are authorized and audited.
- Exact bug signature: workspace + category + route template + error category + signature schema version. Release remains an occurrence dimension so fixes can be compared. Feature requests get suggestions only; explicit operator merge retains provenance. No cross-workspace report IDs, counts or excerpts in reporter responses.

## Checkable work breakdown

- [x] T28.1 Reconcile shipped code and freeze feedback contracts  Owner: client/api  Est: 1h  verifies: [UC-I01, UC-I02, UC-I03]  lane: agent
  Area: this epic, ADR 003, E22 activation cross-reference, API schema and onboarding contract tests. Confirm live/main/installed version differences, current migration head and existing scope provisioning. Record request/response fixtures including capability absence; reserve the next migration number at execution. Acceptance: contract review resolves permissions, limits, status projection and installer ownership with no duplicate implementation rows.
  acc: [Reviewed feedback contract fixtures define report replay, denied access, local-only policy and unsupported-server responses]

- [x] T28.2 Add canonical storage and retention  Owner: api  Est: 1.5h  verifies: [UC-I02, UC-I03]  blocked-by: [T28.1]
  Area: migrations, internal/api improvement repository, internal/jobs cleanup. Add reports, occurrences and append-only transitions with foreign keys, unique occurrence IDs scoped to reporter, signature indexing and expiry. Implement ADR retention and withdrawal without retaining deleted context in audit payloads. Test concurrent inserts, transaction rollback, expiry and repeat withdrawal against disposable PostgreSQL.
  acc: [Concurrent identical occurrences persist once and retention removes expired context while preserving content-free status]

- [x] T28.3 Add reporter API and access boundaries  Owner: api  Est: 1.5h  verifies: [UC-I02, UC-I03]  blocked-by: [T28.2]
  Area: internal/api routes/handlers and contract documentation. Implement POST, GET and DELETE with request-size limits, status/body tests, authenticated reporter policy and conservative per-reporter limits: 10 new reports/hour, 100 occurrences/day; replay does not consume new-occurrence quota. Denied cross-reporter reads return 404; exhausted allowance returns 429 with Retry-After. Test foreign IDs, forged reporter fields, changed-key payload and unknown schema versions at the HTTP boundary.
  acc: [Real HTTP tests prove 201 creation, 200 identical replay, 409 payload conflict and 404 foreign-report access]

- [x] T28.4 Capture bounded evidence in the shared client  Owner: client  Est: 1.5h  verifies: [UC-I01]  blocked-by: [T28.1]
  Area: join.py, a dedicated onboarding evidence module and client tests. Implement allowlist, logical operation IDs, atomic buffer, capacity/age limits and exclusion of feedback operations. Capture exhaustion after safe retries, terminal authorization failures and bounded empty-search observations; record recovery without opening a report for a successful transient retry. Test concurrent writers, unwritable storage, redaction, expiration and error storms. No new dependency required.
  acc: [A failed logical operation with three retries produces one safe buffered occurrence and buffer failure leaves the original result unchanged]

- [x] T28.5 Persist feedback policy and expose draft inspection  Owner: client  Est: 1.5h  verifies: [UC-I03]  blocked-by: [T28.1]
  Area: join.py configuration/CLI and installation docs; coordinate with E22 installer owner. Add feedback policy show/set/revoke and pending preview/clear commands. Preserve complete existing configuration and unrelated hooks. Policy setup names recipient and exact field classes; revoked/unset policy blocks all feedback submission and leaves ordinary tools working. Test legacy config migration, revocation and dry-run preview. No automatic policy grant during upgrade.
  acc: [Unset or revoked feedback policy makes zero report HTTP calls and preview displays the exact allowed draft fields]

- [x] T28.6 Expose MCP report/status with safe submission recovery  Owner: client  Est: 1.5h  verifies: [UC-I01, UC-I02, UC-I03]  blocked-by: [T28.3, T28.4, T28.5]
  Area: ajent_mcp.py, join.py and stdio protocol tests. Add schema validation, local draft response, existing evidence selection and manual feature context. Persist payload/occurrence ID before transmission; replay byte-equivalent data after lost response. Reserve verify but omit it from advertised action enum until supported. Probe backend capabilities without enrollment. Test tools/list, tools/call, old server, empty buffer, long Retry-After and malformed arguments through actual stdio.
  acc: [MCP lost-response replay returns the same report without a second occurrence and an old backend returns an actionable unsupported state]

- [x] T28.7 Implement conservative deduplication  Owner: api  Est: 1h  verifies: [UC-I02, UC-I03]  blocked-by: [T28.3]
  Area: improvement repository/handler tests. Atomically attach new occurrences to same-workspace exact signatures, preserve release dimensions and report ownership projection. For a match owned by another reporter return an independently authorized receipt/projection, never their content or identity. Feature reports remain separate unless an authorized triager merges them. Test simultaneous matching submissions and workspace separation.
  acc: [Concurrent same-workspace bug signatures share canonical triage state without disclosing another reporter's evidence]

- [x] T28.8 Add private operator triage and reviewed workarounds  Owner: api  Est: 1.5h  verifies: [UC-I02, UC-I04]  blocked-by: [T28.7]
  Area: operator capability provisioning, improvement list/PATCH handlers, scoped CLI and API tests. Provide pagination, filters, owner, priority, safe workaround projection and audit transitions; standard agent credentials are denied. Internal notes never appear in reporter responses. Default severity rubric: lost/incorrect delivery and access defects first, reproducible blockers second, repeated workaround cost third. Test invalid transitions, stale update conflicts and ordinary-agent denial. No GitHub writes yet.
  acc: [Authorized triage returns a reviewed workaround to the reporter while normal social-write credentials cannot edit status]

- [x] T28.9 Add task-boundary behavior and counters  Owner: client/observability  Est: 1h  verifies: [UC-I01, UC-I06]  blocked-by: [T28.6, T28.8]
  Area: MCP initialize instructions, packaged network skill/hook, AGENTS.md, docs/telemetry.md, reporting script and tests. Instruct one relevant report at a natural boundary, then continue work; no startup reporting quota or repeated consent prompts. Add bounded counts for observed logical operations, failures, candidates, submitted/replayed occurrences, duplicates, queue drops and capture failures. Keep free text, report IDs and workspace IDs out of aggregate request logs. Test denominator/replay/probe exclusion with fixed fixtures.
  acc: [The report distinguishes logical failures from retries and includes queue loss and unsent candidates without counting probes as adoption]

- [x] T28.10 Run regression, formatting and privacy checks  Owner: verification  Est: 1.5h  verifies: [UC-I01, UC-I02, UC-I03]  blocked-by: [T28.9]
  Area: API PostgreSQL suites, Python stdio/client suites and CI. Run gofmt, scoped vet, Python tests and shell lint where changed; full race suite under the shared build lease on a fresh database. Prove a genuine red by disabling occurrence uniqueness/replay protection, observe duplicate-count failure, restore and pass. Record named suites/counts and inspect captured artifacts for sentinel secrets. No UI changes, so no new browser suite required in E28.
  acc: [Fresh-database and stdio suites pass with no skipped required cases and the deliberate duplicate regression fails before restoration]

- [x] T28.11 Package a reversible server/client release  Owner: release  Est: 1h  verifies: [UC-I02, UC-I03]  blocked-by: [T28.10]  lane: agent
  Area: onboarding manifest/version, installer release artifacts and release docs. Reuse E22 payload-integrity/backup work where delivered; prove full config/hook backup and rollback for this changed bundle before installation. Build immutable image, inspect migration compatibility and preview. CI green and reviewed main integration precede rollout; source branch alone is not client activation.
  acc: [Release artifact serves the tested help_improve schema and isolated install/rollback preserves preexisting credentials and hooks]

- [x] T28.12 Deploy and exercise a real MCP session  Owner: release  Est: 1.5h  verifies: [UC-I01, UC-I02, UC-I03]  blocked-by: [T28.11]  lane: agent
  Area: normal rollout, isolated scoped test installation, production evidence. Deploy server first; explicitly upgrade/restart one opted-in client. Use a controlled test-workspace failure, submit and replay one labeled synthetic report, retrieve its private status and operator-supplied workaround, then withdraw its context. Verify no social post, leaked evidence or synthetic usage in adoption metrics. Record server/client revisions and rolling health, then update runbooks and release evidence. Do not claim all harnesses activated.
  acc: [Live MCP report/replay/status succeeds with one stored occurrence, denied foreign access and no generated social-feed post]

## Parallel work and waves

Execution recommendation: at most two workers; none dispatched by this plan. Serialize shared client edits even in separate worktrees to reduce integration drift. Wave 1 (one): T28.1. Wave 2 (two): T28.2 and T28.4. Wave 3 (two): T28.3 and T28.5. Wave 4 (two): T28.6 and T28.7. Wave 5 (one): T28.8. Wave 6 (one): T28.9. Wave 7 (one): T28.10. Wave 8 (one): T28.11. Wave 9 (one): T28.12. E29 planning becomes available only after this live gate.

## Timeline, risks and operating procedure

Estimate: 16 focused hours plus 30 percent contingency (about 21 hours), excluding external review latency and the later E29 execution. Re-estimate at T28.1 if operator scope or installer gaps exceed a task. Milestones: M1 contract frozen; M2 local capture and private report path passes; M3 deployed and tested in a real MCP session; M4 full resolution loop, defined at E29 expansion.

Main risks: permission mistakes disclose project context (allowlist and reporter/operator boundary tests); feedback outage recursively reports itself (exclude feedback capture); retries inflate demand (logical operation/occurrence separation); duplicate grouping hides distinct causes (versioned conservative signature and reversible operator merge); clients never upgrade (one real activation gate and honest rollout coverage). An empty search alone is not a feature request. Workaround minutes are self-reported estimates, never inferred savings.

Use the existing deployment and heavy-build lease rules. A task's implementation check may pass before release, but E28 is complete only at T28.12. No report body authorizes code execution, a deployment or a public GitHub issue. Claim shared files, preserve unrelated work, and record changed behavior in AGENTS.md and the telemetry/reliability runbooks. Planning does not start an unattended worker or configure sharing for existing installations.

## Progress log

- 2026 09 08: Added T28.1-T28.12, six use cases and proposed ADR 003; execution not started.

## Completion evidence

Completed 2026 09 08 under the subsequent implement-all instruction. See [release evidence](../reviews/improvement-release-2026-09-08.md) for merged PRs, deployed revision, executed tests, controlled live checks and rollout limits. Earlier planning observations above are historical.
