# E30 -- Run improvement operations independently of the operator Mac

fidelity: executable

Acceptance: Production replicas process reviewed reports through one durable ownership protocol. Instance loss and an ambiguous GitHub response cannot cause blind duplicate creation. The Mac scheduler is stopped only during a verified cutover; cloud heartbeat, stalled-work alerts and rollback are exercised live. All implementation rows require T30.12 before this epic is shipped.

## Context and discovery

Engineering and operations, 2026 09 08. The user requested a plan for the remaining improvement-loop work. Reuse deployed E28/E29 and ADR 003. ADR 004 chooses the existing Go/PostgreSQL job infrastructure, not another service platform. Graph discovery was attempted first; its snapshot predates the improvement release and has no improvement community, so source inspection supplied the missing seams. Source reviewed from the merged release checkout, not the unrelated dirty root branch. Refresh main, production and open PRs at execution.

Current seams: scripts/improvement-coordinator.py skips new reports and uses a local lock/journal; scripts/improvement-triage.py implements sanitized marker projection and ambiguous-create recovery; internal/jobs/jobs.go is the existing scheduler; internal/api/improvements_operator.go owns authorized state transitions; infra/index.ts and infra/rolling.ts define production secrets and alerts. Do not add network/model work to the existing synchronous retention chain. API list/PATCH, deduplication, reporter receipts and typed verification remain the base.

## Use case summary

UC-O01 (P0): reviewed reports continue processing after host failure without duplicate public issues. UC-O02 (P0): operators see and receive actionable stale-worker evidence. Both are PARTIAL. Durable catalogue: improvement-operations-usecases.json. Scope here excludes automatic triage of new reports, which is E31.

## Work breakdown

- [ ] T30.1 Reconcile release state, issue ownership and activation overlap  Owner: coordinator  Est: 60m  delivers: [dated source/live/issue reconciliation and narrowed E22 overlap]  lane: agent
  Area: docs/reviews/improvement-operations-discovery.md, E22, current release evidence, existing GitHub issues. Inspect current main and loaded/live versions; search issues including chief's work before adding any. Record reuse, missing behavior and owner per relevant E22 row. Preserve statuses unless the entire row has evidence; use the overlap notes below as discovery, not a completion verdict. Acceptance: every selected gap has one owning row and every proposed issue either links an existing issue or explains why none matches. Read-only discovery does not send messages or create issues.
- [ ] T30.2 Freeze durable jobs, cutover and operation contracts  Owner: api  Est: 90m  verifies: [UC-O01, UC-O02]  blocked-by: [T30.1]  lane: agent
  Area: docs/contracts/improvement-operations.md, ADR 004, operator API schemas. Specify job key (repository, report, operation, policy/revision), cursor fairness, lease generation, retry states, exact GitHub repository and credential provisioning, cancellation on withdrawal, retention and local-journal import. Defaults: tick 60s, at most 10 jobs/pass, one external action at a time/instance, 120s lease, 30s renewal, 15s request timeout. Set projection heartbeat stale at 5 minutes and oldest runnable-work warning at 15 minutes. Acceptance includes real-boundary HTTP fixtures for added operator routes and incompatible legacy-client behavior.
  acc: [contract fixtures distinguish claimed pending sent ambiguous completed and cancelled jobs and define denied operator responses]
- [ ] T30.3 Persist jobs, intents and bounded history  Owner: api  Est: 90m  verifies: [UC-O01]  blocked-by: [T30.2]
  Area: next available migration, new internal/improvements job repository and PostgreSQL tests. Unique operation keys, attempt count, next-attempt time, lease owner/generation and pre-send intent. Index runnable work and age; completed history expires after 90 days. Preserve unresolved sent/ambiguous marker tombstones until manually reconciled so retention cannot enable duplicate creation; contain no reporter context. Withdrawal cancels unstarted effects. Test unique inserts, rollback, retention and withdrawal at the SQL boundary.
  acc: [concurrent insertion of the same operation yields one durable job and retention never makes an ambiguous create eligible for a new create]
- [ ] T30.4 Implement renewable fenced claims and reclaim  Owner: api  Est: 90m  verifies: [UC-O01]  blocked-by: [T30.3]
  Area: internal/improvements claim/renew/finish methods and real PostgreSQL concurrency tests. Atomically select due jobs, renew with generation checks, reclaim expired ownership and reject late finish. No long transaction during external calls. Test two claimants, expired owner, process-loss equivalent and cancelled report; claim loss after sent intent remains ambiguous, not retryable create.
  acc: [two concurrent workers cannot own one generation and a reclaimed job rejects the previous worker's completion]
- [ ] T30.5 Port sanitized projection and authenticated GitHub adapter  Owner: integration  Est: 90m  verifies: [UC-O01]  blocked-by: [T30.2]
  Area: new internal/improvements/github.go and tests; reuse scripts/improvement-triage.py contract, fixed projection, marker and exact repository validation. Use scoped production credentials, no shell gh dependency in the application. Freeze supported credential acquisition against current official provider documentation during implementation. Fake HTTP server tests assert methods, status/body, authentication refresh, pagination, rate limit and timeout classification; sentinel private context never appears in outbound payload.
  acc: [GitHub boundary tests project only allowlisted content and distinguish definite rejection from ambiguous creation]
- [ ] T30.6 Join durable intents with recoverable external effects  Owner: integration  Est: 90m  verifies: [UC-O01]  blocked-by: [T30.4, T30.5]
  Area: internal/improvements projection dispatcher and fault tests. Commit sent intent before HTTP; attach exact existing marker before create; persist successful link using current ownership/version. An ambiguous create can only search/attach/escalate, never retry create blindly. Cap definite transient retries at five with bounded exponential delay; preserve dead-letter reason and operator retry only for safe states. Test crash before send, response loss, success-before-link and takeover during request.
  acc: [a lost create response followed by worker replacement produces one issue or an explicit ambiguous record and never a second create request]
- [ ] T30.7 Run isolated bounded production passes  Owner: runtime  Est: 60m  verifies: [UC-O01, UC-O02]  blocked-by: [T30.6]
  Area: internal/jobs/jobs.go, new improvement worker entry, cmd/ajentd configuration. Separate context/ticker from publisher and cleanup; enable flag defaults off until configured. Durable pagination/checkpoint prevents starvation beyond first page. Bound run duration and shutdown; no model processing yet. Test >50 eligible reports, poison first row, cancellation and dependency failures while publisher continues.
  acc: [a blocked projection does not stop event publication and later eligible reports are processed across bounded passes]
- [ ] T30.8 Expose worker health and actionable aggregate logs  Owner: observability  Est: 90m  verifies: [UC-O02]  blocked-by: [T30.7]
  Area: operator health route/schema, improvement metrics, docs/telemetry.md and reporting script. Report last completed pass, oldest due job, lease/retry/ambiguous/dead-letter counts and disabled/degraded/healthy state. Real HTTP tests assert 200 body for operator, denial for ordinary credentials, and stale versus empty. Aggregate logs carry state/latency/revision only; internal IDs stay private.
  acc: [operator health distinguishes disabled stale degraded and healthy-empty states without exposing private report content]
- [ ] T30.9 Wire production credentials and stalled-worker alerts  Owner: infrastructure  Est: 90m  verifies: [UC-O02]  blocked-by: [T30.8]
  Area: infra/index.ts, infra/rolling.ts, infra/userdata.ts and infrastructure tests. Use existing secret/alert resources, narrow repository credential access, missing-data alarms and recovery notifications. Configuration missing or revoked is degraded, not success. Tests assert IAM scope and alarm missing-data behavior; preview contains no unrelated resource replacement. T30.12 must prove actual delivery to the configured operator destination, not only alarm resource creation.
  acc: [infrastructure tests detect stalled heartbeat and restrict worker secret access to configured resources]
- [ ] T30.10 Implement restartable local-journal cutover and rollback  Owner: integration  Est: 90m  verifies: [UC-O01]  blocked-by: [T30.6, T30.9]
  Area: scripts/improvement-coordinator.py, install/uninstall script, new journal import CLI and runbook. Preview import counts without private output; stop old launchd job, confirm no local pass active, import pending/linked/ambiguous markers idempotently, enable cloud worker. Refuse cloud activation on conflicting unresolved journals. Rollback disables cloud effects before restoring local ownership using an exported reconciled journal; never run independent writers. Real CLI/API tests cover interrupted import, repeated import and denied caller.
  acc: [an interrupted cutover resumes without losing pending intents and refuses simultaneous uncoordinated local and cloud writers]
- [ ] T30.11 Run boundary, fault and formatting gates  Owner: verification  Est: 90m  verifies: [UC-O01, UC-O02]  blocked-by: [T30.10]
  Area: changed Go/Python/infra suites and CI. Run gofmt, vet, relevant Python/infra lint and fresh PostgreSQL race tests under the mini lease. Disable a fencing check to demonstrate a genuine failing takeover test, restore and pass. Record named suites/counts, zero hidden DB skips, API status/body and sentinel leak checks. No repeated full suite without new evidence.
  acc: [fencing and ambiguous-effect fault suites pass on fresh PostgreSQL and disabling fencing produces the predicted failure]
- [ ] T30.12 Deploy, cut over and verify recovery live  Owner: release  Est: 90m  verifies: [UC-O01, UC-O02]  blocked-by: [T30.11]  lane: agent
  Area: existing deployment, private operator activation, docs/reviews and AGENTS.md. Prepare exact CI artifact and rollback, acquire deployment lease, deploy, then perform controlled synthetic cutover and process-loss/ambiguous-outcome checks. Verify both application replicas, no duplicate issue, continued inbox publication, fresh cloud heartbeat and alert/recovery delivery. Confirm local scheduler is stopped. Observe 24 hours with no unexplained overdue work; report actual samples and outages. Record missing credentials/destination as unmet gate, never healthy. No routine social messages.
  acc: [production processes synthetic reviewed work after worker loss with one linked issue and receives stalled-worker and recovery alerts while the local scheduler is stopped]

## Waves and milestones

Ops waves (exact worker counts): 1 (one) T30.1; 2 (one) T30.2; 3 (two) T30.3/T30.5; 4 (one) T30.4; 5 (one) T30.6; 6 (one) T30.7; 7 (one) T30.8; 8 (one) T30.9; 9 (one) T30.10; 10 (one) T30.11; 11 (one) T30.12. Separate ownership for store and GitHub adapter in wave 3. T31.1 may start after T30.2, but shared runtime/API edits serialize. One production deploy owner and one heavy-build lease.

Estimate: 17 focused hours plus 30 percent integration reserve, approximately 22 hours, plus the 24-hour observation window and any credential setup latency. Re-estimate after the contracts. First milestone is durable local fault tests; second is live cloud operation. No claim of exactly-once GitHub delivery or automatic repair.

## E22 overlap to preserve

Reuse quiet presence, typed errors, inbox acknowledgment/cursors, route telemetry and full-file backup from E28/E29. E22.8 should adopt the shipped inbox through-cursor contract instead of creating a second acknowledgment scheme. E22.16 immutable hashes/pinning/preview and E22.17 interrupted promotion remain open. E22.18/.19/.23 still require real two-harness activation. E33/E35 refine those owners; they do not duplicate their implementation. T30.1 records per-row evidence before closing anything.

Progress: 2026 09 08 planned T30.1-T30.12; no new worker, cloud change or external publication performed.
