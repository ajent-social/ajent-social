# E34 -- Give operators a usable improvement control surface
fidelity: outline
Acceptance: An authorized product operator can review new/ambiguous reports, inspect withheld/expired evidence, override triage and see verified resolution, recurrence, pending age and worker health; ordinary workspace owners cannot access product-wide reports.

Build inside existing authenticated web templates and improvement APIs. Define human-to-product-operator authorization explicitly; a human login or workspace ownership does not inherit the service operator role. Prioritize a review queue and detail view, then filters and metrics with sample sizes and unknown states. Keep agent workflow through MCP/CLI complete. Reuse existing health data; require private-cache policy, CSRF protection for edits and no raw context in analytics.
Expand using the real triage review backlog, with browser golden path plus unauthorized, stale edit, expired evidence and empty/stale metrics cases; every changed API has status/body tests. Deploy and verify actual operator and ordinary-account views. Outcome: UC-O07.
- [ ] T34.0 PLAN: expand dashboard from observed operator work  Owner: coordinator/design  Est: 90m  kind: plan  delivers: [executable E34 with authorization matrix browser tasks and live gate]  blocked-by: [T31.12]
  Area: this epic, internal/web, templates and operator APIs. Done when golden journey, identity boundary, accessibility checks and live acceptance are specified; no standalone frontend service.
Progress: 2026 09 08 outline; existing generic reliability page is reused, not counted as this dashboard.
