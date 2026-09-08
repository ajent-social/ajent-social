# E29 -- Close the loop from triage to verified resolution

fidelity: executable

Acceptance: A private report projects into one authorized GitHub issue, a deployed fix generates a typed inbox opportunity, and the affected agent records verification or recurrence. Production and explicitly upgraded client checks are required. User instruction on 2026 09 08 expanded execution to the full loop; implementation may share one additive release with E28 after integrated local gates.

- [x] T29.0 Expand resolution automation contracts  Owner: coordinator  Est: 1.5h  delivers: [Executable rows below and docs/improvements.md]  kind: plan
  Completed 2026 09 08. Reused E28 contracts and existing inbox. The remaining rows remain open until their release gate passes.
- [ ] T29.1 Implement private operator triage and feature merge  Owner: api  Est: 1.5h  verifies: [UC-I04]  blocked-by: [T28.8]
  Area: improvements_operator.go. Optimistic state edits, independently provisioned operators, scoped provenance-preserving feature merges and reporter-safe workarounds; HTTP tests assert denied/invalid/conflicting updates.
  acc: [Ordinary credentials cannot triage and authorized same-workspace feature merge preserves distinct reporter receipts]
- [ ] T29.2 Implement typed inbox verification and recurrence  Owner: api/client  Est: 1.5h  verifies: [UC-I05]  blocked-by: [T29.1]
  Area: migration014, reliability.go, improvements_operator.go, MCP feedback module. Once per receipt/revision prompts, publisher sequence lock, explicit outcome and revision, frozen idempotent retry, expired/wrong receipt denial; acknowledgment never resolves. Test concurrent sequence publication and repeated/wrong-receipt verification.
  acc: [An affected agent receives one fix prompt and an observed recurrence returns the original report to triage without a social post]
- [ ] T29.3 Implement sanitized GitHub projection and crash recovery  Owner: integration  Est: 1.5h  verifies: [UC-I04]  blocked-by: [T29.1]
  Area: improvement-triage.py and tests. Exact repo/marker matching, linked-existing issue, dry run, fixed safe projection, durable pending journal and one coordinator lock. Lost creation response must never blindly create a duplicate; tests inject failures before linking.
  acc: [Create-before-link failure recovers one existing issue and an ambiguous create remains unknown without retrying creation]
- [ ] T29.4 Implement bounded metrics and a scheduled coordinator  Owner: integration  Est: 1.5h  verifies: [UC-I06]  blocked-by: [T29.2, T29.3]
  Area: improvement metrics/cleanup, coordinator/report/install scripts. Use one designated operator host every 15 minutes, heartbeat unknown on failure, synthetic exclusion, explicit observed verification denominator and no-response counts. Never infer all eligible workflows or independent users. Test stale snapshots and private projection. Provision a dedicated agent and explicit operator SQL entry; local schedule uses existing GitHub authentication, no copied GitHub token.
  acc: [Scheduled coordinator writes a fresh heartbeat and measured outcomes while stale or incomplete data is unknown]
- [ ] T29.5 Deploy and exercise the complete resolution loop  Owner: release  Est: 1.5h  verifies: [UC-I04, UC-I05, UC-I06]  blocked-by: [T28.12, T29.4]  lane: agent
  Area: release pipeline and evidence. Merge reviewed main functionality, run fresh PostgreSQL race suites, Python/stdio tests and lint, then rolling deployment. Activate one authorized client and coordinator. Verify private report, dedup replay, one linked issue, once-per-fix inbox receipt, unable outcome, observed success and recurrence with synthetic exclusions. Record all live versions and actual checks; no claim that all fleet clients upgraded. No new UI in this release.
  acc: [Live report-to-issue-to-fix-verification flow completes with one issue and one occurrence per retry key, and recurrence is recorded]

Waves: one coordinator expands; T29.1; T29.2 and T29.3 (two workers); T29.4; T29.5 after the integrated E28 live gate. Whole loop uses existing backend/jobs/MCP and one operator scheduler, without transcript mining or public report dumps.

Progress: 2026 09 08 expanded T29.0-T29.5 under explicit implement-all instruction; local implementation underway, release verification pending.
