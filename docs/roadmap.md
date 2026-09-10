# Ajent roadmap

## In flight

- 2026 09 08: E30 job store and GitHub adapter dispatched in isolated worktrees. Current main baseline build passed. T30.1/T30.2 and T31.1 contracts recorded; repository-scoped production GitHub credential is not yet configured. Existing local coordinator remains active until a verified cutover.

## Planned

- 2026 09 08: [E30 durable operations](plans/E30-durable-improvement-operations.md) and [E31 proactive triage](plans/E31-proactive-improvement-triage.md) define 24 executable rows with production recovery, alert, shadow and canary gates. E32 deployment linking, E33 fleet activation, E34 operator dashboard and E35 useful participation retain the remaining scope as gated outlines in [the master](plan.md). [ADR 004](adr/004-bounded-improvement-automation.md) chooses durable database ownership and bounded triage proposals. E22 implementation overlap remains owned there; completed E28/E29 evidence is retained in split files, design/devlog and the release record. Planning only; no new runtime activated.

- 2026 09 08: [Work-derived improvement reports](plans/E28-help-improve.md): 12 executable tasks for safe local evidence, standing feedback policy, private idempotent reports, conservative deduplication, triage/workarounds and a real production MCP check. [E29](plans/E29-improvement-resolution.md) gates GitHub projection, typed inbox fix verification and outcome metrics on that live milestone. Six planned use cases; [ADR 003](adr/003-work-derived-improvement-reports.md) records the proposed contract. First milestone estimate about 21 focused hours including contingency; no implementation or client policy change performed by this entry.

- 2026 09 08: [Moltbook execution program](plan.md): 61 executable preparation tasks across proof, activation, founding community, public stories and organic distribution; two gated later planning passes for cohort execution and learning. [Pilot protocol](marketing/moltbook-pilot-protocol.md) defines independent-team denominators, repeat/cross-owner outcomes, timing and stop rules. [ADR 002](adr/002-checked-exchange-growth.md) is proposed. Existing launch and memory work remain linked; prior master preserved exactly in docs/legacy/production-plan-2026-09-08.md. No campaign or engineering implementation performed by this planning entry.

- 2026 09 07: [Local agent memory](plans/local-agent-memory.md): extend the Go aj client, preserve Python installation compatibility, integrate a supported Serenity single-writer host, and prove offline persistence across sessions. Installation/dogfood follows the local preview; paid managed sync and team memory remain evidence-gated. Direction recorded in [ADR 001](adr/001-local-agent-memory.md). Planning only; no memory feature is shipped by this entry.

- 2026 09 07: [24-hour launch and paid-work scope](plans/launch-and-paid-work.md): original proposal for public DNS/TLS, alert delivery, an apex demonstration, invited access and the existing consulting path. Reconcile its historical missing-feature claims with current deployment records and HQ's newer September 8 staged-demo packet before executing remaining work. This entry itself records no deployment or outreach.

## Existing evidence

- [Deployment record](deployment.md) documents the AWS backend, CI and recovery checks already completed. Follow its latest entries rather than the archived MVP plan.

## Improvement loop shipped — 2026 09 08

E28/E29 implemented and deployed. See [release evidence](reviews/improvement-release-2026-09-08.md) for tests, controlled verification and activation coverage.
