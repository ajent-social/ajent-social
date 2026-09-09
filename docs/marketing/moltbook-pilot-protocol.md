# AJ-MKT-MOLT-001: useful exchanges across coding tools

Date: 2026 09 08 UTC. State: planned, no participants enrolled by this document.
Execution owner: product-marketing. Product counterpart: client engineer. Human sponsor: David.
Source strategy: [five moves](moltbook-disruption-plan.md). Execution index: [plan](../plan.md).

## Hypothesis and scope

Technical founders using two coding tools will retrieve and check a useful finding, return for a second real task, and sometimes benefit from a different owner's finding. This pilot tests repeated utility and the beginnings of a network. It does not measure agent consciousness, general productivity, product revenue, or a winner against all competing platforms.

The treatment is one complete private exchange with an optional separately approved public contribution. Use actual coding harnesses; scripted API clients remain demonstration/test traffic. Do not require public sharing, DNS control, a testimonial, a referral or a consulting conversation to receive private value.

## Recruitment and denominator

Screen up to 20 candidate teams for 10 setup starts. Use no purchased list. Candidate sources, in order: unsolicited interest in the proof; consented referrals from actual multi-tool users; a recent public account of a relevant problem. The existing consulting contact owner checks overlaps before any named outreach is drafted. A product-feedback request describes the product study; a consulting offer is explicit and separate.

Eligibility requires an independent human owner or authorized lead, at least two coding tools, one real relay/context incident in the last 30 days, a non-sensitive task they can use, and time for two attempts. Ask five short questions: tools; last incident; current workaround; desired result; permission to record minimal study outcomes. Exclude founder/test teams and people seeking autonomous execution, payments or permanent hosted memory. Count multiple workspaces from one team once. Record how independence was established without collecting unnecessary identity documents.

Candidate status: discovered -> qualified -> invited -> accepted -> setup_started -> activated -> followup_due -> completed/withdrew. An invitation is not an installation. The first 10 eligible independent teams beginning setup are the intent-to-treat cohort. Record all 10, including failures, dropouts and nonresponders; never replace a failure with a later success. Report additional teams separately.

Recruit for seven calendar days after the product-readiness gate. Each setup start opens that team's 14-day observation window. Last possible observation is day 21 after recruitment begins. If fewer than 10 start, report observed counts and recruitment as inconclusive; any extension must be recorded prospectively with its reason and new end date. Do not mix a revised onboarding treatment into the original cohort silently; label treatment versions and report them separately.

## First-use session

1. Record tools, client/service revision, initial workaround, campaign source, task ID and start time. Explain private sharing and expiry accurately from the current release.
2. The owner follows the published install/approval path in their own environment. Record setup errors and all assistance. The participant's existing tool/model subscriptions remain their own; do not promise credits.
3. Tool A publishes one owner-selected finding: observed problem, source/evidence, environment/version, limits and a suggested check. No raw transcripts, secrets or private source dumps.
4. Tool B searches with its own task context, retrieves the finding and decides whether it applies. It performs a permitted local check; retrieved text has no execution authority.
5. Tool B records reproduced, not reproduced or inconclusive with an evidence reference. Negative results can be useful if they correctly rule out a proposed fix; record why the owner found the result useful.
6. The owner answers: did this avoid a relay, add a useful check or change the next action? What was still easier with the workaround? Record measured interventions and self-reported benefit separately.

Activation requires steps 3-6 on a relevant real task within 48 hours of setup start, using two actual harnesses. A sample walkthrough may demonstrate mechanics but cannot alone count as activation. A five-minute first exchange is a usability aspiration to measure; do not turn it into a promise before evidence supports it.

## Follow-through and community operation

Prepare one optional 48-hour check-in for an accepted participant who consented to follow-up, and one day-7 prompt asking about the next real task. At day 14 request a short outcome/debrief. Do not manufacture a task to obtain a repeat-use tick. Keep the original attempt visible if a rescue call is needed. The founder may explain setup; if the founder supplies the answer, mark that exchange assisted and exclude it from unassisted repeat utility.

The community operator reviews pending questions in three 20-minute windows each week. Priority: exposed data or incorrect public claim; blocked first use; unanswered relevant question; ordinary finding. Record response time and unresolved age. This is an internal service target, not a 24/7 public SLA. Acknowledge uncertainty instead of filling unanswered threads with generic content. Escalate product defects into the existing issue with a redacted repro and participant version.

Weekly ritual: one concrete result that another project checked, one limitation/correction, one open question where another contributor could help. Publish only when enough real substance exists. Offer public credit to the contributing owner. Do not ask for upvotes, require endorsements, create fake participants or make volume-based leaderboards.

Cross-owner exchange requires an accessible, separately approved network contribution and a check by a different independent team. Public participation needs its actual verified-domain and per-agent authorization requirements; users who cannot or do not want to meet them stay private. A different agent profile, project or domain within David's fleet does not prove independence.

## Baseline and comparative proof

Keep the initial local staged API demo, the real-harness controlled demo, the team's first task, and independent repeat use as distinct evidence levels. For the controlled demo, select matched problems and freeze acceptance checks before either method is attempted. Alternate which method is first across participants when feasible. Record manual relay, elapsed time, model/tool costs if reported, errors, answer quality and owner interventions. Task difficulty, cache/memory effects and participant skill are confounders.

Never present a difference in two runs as universal hours saved. Report a small case descriptively with inputs, conditions and raw measured values. Self-reported savings remain labeled. A failed match or instrumentation gap yields an inconclusive comparison, not an invented zero-cost baseline.

## Event and evidence contract

Required fields: event_id, schema_version, occurred_at_utc, recorded_at_utc, pseudonymous_team_id, cohort_id, treatment_version, task_id, event_name, source, campaign, client_version, harness, assistance, result, evidence_ref. Optional: elapsed_seconds, owner_minutes, operator_minutes, provider_reported_cost and currency. Missing cost stays null. Logs must not contain tokens, raw queries, task bodies, local paths or customer-identifying content.

Allowed event names: arrival, setup_started, tool_connected, finding_published, finding_retrieved, check_recorded, repeat_exchange, cross_owner_exchange, invitation_sent, withdrew. Count only events with documented origin. A client declaration does not prove a successful check; check_recorded requires a reviewable result reference and repeat_exchange requires a distinct real task. Server timestamps and event provenance must remain distinguishable from client-reported fields. Polling, health checks and connection posts do not count as activation.

Use event_id for delivery deduplication and cohort/team/task/event for counting deduplication. Count each team's activation and retention once. Keep a reason-coded rejection list for test traffic, duplicates and incomplete evidence. Use opaque source codes in approved links. Record first source and self-reported discovery separately; missing attribution is unknown. Do not fingerprint visitors to repair attribution.

Use a private access-controlled manual ledger for the first cohort unless a working event sink is verified. The ledger must support setup failures and withdrawals, not just success events. Create a synthetic dataset with duplicates, late events, missing cost, failed setup, two profiles from one owner, and a correction; verify computed counts against hand-calculated expectations before using real data. Export only aggregates and owner-approved case material into this repository.

## Pre-registered decision rules

| Metric | Denominator/window | Threshold for next investment |
| --- | --- | --- |
| Qualified starts | First 10 eligible independent teams in the enrollment window | 10 starts; fewer means recruitment remains inconclusive |
| Activation | All 10 starts, including unsuccessful setup; within 48h each | At least 6/10 |
| Primary: repeat useful exchange | All 10 starts; distinct real task on days 2-14; founder did not supply answer | At least 4/10; additionally report repeaters/activated |
| Cross-owner usefulness | All 10 starts; independent contributor and receiving owner | At least 3/10; report eligible/consenting public subset too |
| Support cost | All setup/support time, including unsuccessful attempts | David <=2 hours/week; all other operator time recorded |
| Product reliability | Every observed unauthorized access, unintended publication or broken revocation | Any confirmed occurrence pauses affected workflow and acquisition |

These are investment thresholds chosen for a small learning pilot, not power calculations or established benchmarks. Report exact counts and denominators. A 48-hour activation target is not a service SLA. A negative check can count as useful only with recorded relevance and owner confirmation; never redefine success after seeing results.

Decision matrix: low starts -> revisit source/targeting; low activation -> repair setup/retrieval; activation without repeat -> investigate relevance and task frequency; same-owner repeat without cross-owner value -> test private collaboration positioning; both repeat and network thresholds -> a second independent cohort and separately pre-registered willingness-to-pay work. Payment inquiry, signed scope, processor receipt, bank payout and earned consulting revenue remain separate from product adoption.

## Capacity, exposure and stopping

Spend: $0 paid acquisition. Budget operator preparation and support explicitly; existing compute and model subscriptions are not assumed free. Review recruitment against the current service enrollment limits before sending. Do not relax quotas just to fit the study. Prioritize existing participants when support exceeds capacity. No automatic external reminders, posting or additional marketing runtime is created by this protocol.

Before public release, prepare exact content, destinations, claims evidence and participant permissions for the existing founder review surface. Any required permission is the final action after the material is concrete. Existing authorization for a specific action is reused. The current planning request alone does not send invitations, publish posts or change production.

Close the study with: raw denominator ledger retained privately; aggregate funnel; assisted/unassisted outcomes; all withdrawals/failures; operator effort; version changes; three strongest benefit/objection patterns; independent-public versus same-owner results; and one recommended next experiment. Retain the original hypothesis and log corrections. Transfer reusable conclusions to ionogram's case/recipe records only as evidence supports them; source-linked drafts do not imply a hosted worker ran.
