# E33 -- Activate supported clients and make upgrade coverage visible
fidelity: outline
Acceptance: Two real supported harnesses safely upgrade, reconnect and expose current tools; the owner can distinguish installed payload, loaded process version, unknown state and stale observation without a fleet-wide activation claim.

Reuse E22.16-.19/.23 for immutable hashes, version pin, preview, staged crash recovery and full rollback. Narrow those existing rows, do not create competing installers. Extend internal/api/reliability.go, the existing reliability page and client capability evidence; one last-observed version per profile cannot prove every concurrent process upgraded. Choose bounded installation/process identity and expiry without collecting project paths. Preserve sharing policy and unrelated registrations; upgrade never silently grants feedback policy or kills active sessions.
Expand with actual two-harness install/reload/rollback, tamper/missing-payload/power-loss tests, HTTP privacy tests, browser golden/unknown-state checks and production coverage verification. Outcome: UC-O06. Missing remote access remains unknown; offer actionable reconnect instructions.
- [ ] T33.0 PLAN: expand fleet rollout using reconciled E22 ownership  Owner: coordinator  Est: 60m  kind: plan  delivers: [executable E33 and reconciled E22 installer rows with one implementation owner]  blocked-by: [T31.12]
  Area: this epic, E22, shared onboarding bundle and reliability surfaces. Done when supported harness matrix, per-process evidence policy, rollout stages, rollback and live gates are specified.
Progress: 2026 09 08 outline; one activated local installation remains the known baseline.
