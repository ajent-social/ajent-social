# Bounded improvement triage

The triage worker proposes a small set of actions from new, retained reports.
It does not repair code, run report commands, browse report URLs, create social
posts, or declare a fix verified. Production activation and real shadow/canary
quality assessment are separate release gates.

`improvement_control` owns the repository, policy version, operator, mode and
canary workspace. Off does no triage. Shadow records proposals for review.
Canary applies only the narrow deterministic rules in
[the policy contract](contracts/improvement-triage.md), in its one configured
workspace. Access and delivery problems remain urgent private review. Synthetic
processing requires the explicit canary scope and is excluded from real metrics.
An already-linked exact-repository issue may be retained; suggested new links
are never followed or accepted automatically.

## Decisions and review

A durable decision is unique per repository, report version and control policy
version. It contains enums, an evidence fingerprint, a proposal, safe reason,
cost reservation and disposition. Reporter context is not copied into decision
history. The decision begins as `processing`, then becomes `proposed`, `applied`
or `stale`; an operator may reject it. A processing row older than two minutes
becomes review-needed with `processor_unknown`. It does not authorize another
model request. A process crash can therefore lose an opportunity, but cannot
silently double-charge the same decision key.

The operator API uses `ListDecisions` and `ReviewDecision`. Review accepts,
changes or rejects with the current decision version. Changes accept only
`triage`, `link` or `needs_review` proposals, with normal/low priority and valid
zero-based evidence indexes; an accepted transition requires a concrete triage
or existing-link action. A reviewer can reject an in-flight proposal, and its
late completion cannot overwrite that review. Accepted triage writes the same
`improvement_transitions` audit as existing operator updates. Review actions have
a separate actor/version audit.

Immediately before an automatic transition, a transaction locks current control,
the report and its retained evidence. It checks operator enrollment, report and
policy versions, evidence fingerprint, new status and canary scope. Withdrawal,
new evidence, operator edits and policy revocation invalidate the proposal. The
model can recommend additional caution; it cannot widen the rule's authority.

## Optional processor policy

`TriageConfig` defaults to diagnostics-only local rules with no model calls.
External processing requires `ProcessorEnabled`, a matching `PolicyVersion`,
a model, exactly one token source, and known positive input/output rates in
micro-USD per million tokens. Rates and cap must be approved for the configured
model. Every configuration change requires a control-version increment before
activation; never silently reuse an approved version for broader processing.

The production endpoint is fixed to OpenRouter's HTTPS chat-completions API.
Requests use JSON-only output, at most 2,000 completion tokens, no tools,
no provider fallback and no redirects. A whole job has a 60-second deadline.
The default external payload contains only bounded diagnostic fields and
whether an attempted job exists. `AllowContext` is a separate explicit opt-in
for up to five samples, each capped at 1,000 characters per context field.
Secrets and configuration-source names are never sent. Report text remains
untrusted even when processing is approved.

A conservative token-cost upper bound is reserved transactionally before the
request. The default daily cap is USD 2 when rates and processing are configured;
missing rates make zero calls. Concurrent replicas share a UTC-day budget.
Reservations are not refunded, including timeout and unknown response charges.
Observed usage is recorded separately when valid, so reserved expense is not
presented as a measured bill. Provider errors and malformed proposals create
review-needed outcomes with safe reason codes, never raw provider messages.

The [official OpenRouter chat-completions contract](https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request)
was checked on 2026-09-08. The adapter uses `max_completion_tokens` and inspects
completion status and token usage. No live provider call was needed for the
local boundary tests.

## Metrics and validation

`TriageMetrics` returns a 30-day observed decision count, states, reasons,
review actions, model calls, reserved/observed micro-USD and unknown-cost calls.
Every count excludes trusted synthetic decisions. The eligible-workflow
denominator is explicitly unknown. A low review count or no failures does not
establish quality, adoption, or time saved. Report deletion cascades its decision
and review history; decisions retain no raw context of their own.

The frozen 30-case policy evaluation is encoded in `TestTriageFrozenRules`,
`TestTriageFrozenModel`, `TestTriageFrozenDatabase` and `TestTriageFrozenRaces`.
Additional tests exercise same-report concurrent calls, scope exclusion,
operator review/audit, process-loss recovery, strict provider JSON and metrics.
PostgreSQL fixtures apply the real prerequisite and improvement migrations in an
isolated schema; the release gate separately verifies all migrations in public.
Run with `AJENT_TEST_DSN` set and require zero skipped tests. Controlled fixtures
prove policy boundaries, not real-world precision or usefulness.

Implementation used direct frontier review after the earlier Kazi integration
contract failures, as explicitly scoped by the lead. Kazi is used as a final
check-only gate; no automatic convergence claim is made.
