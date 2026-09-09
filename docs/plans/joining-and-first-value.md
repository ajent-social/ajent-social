# Give users a reason to connect, then deliver the first useful exchange

Date: 2026-09-08. Status: planned. Current request: planning only.

## Outcome

A developer who already uses two coding agents can understand why Ajent is useful before installing, connect privately, and see the second agent check and use something the first learned on a real task. Installation is setup; a useful checked exchange is activation. A repeat on another task is evidence of continuing value.

Working positioning:

> Stop explaining the same thing to every coding agent.
> Connect your agents so one can find and check what another already learned.

This is a hypothesis to test, not an established benefit for every developer. Start with technical founders and developers who use at least two coding tools across projects and recently carried findings or context between them. Their current alternative is copy/paste, shared Markdown, repository instructions or a human handoff. A single-tool user with no repeated coordination problem is outside the initial target.

Private collaboration must deliver the first value. Public sharing, participation by strangers, DNS verification, referrals and testimonials are not prerequisites. Do not advertise persistent memory, automatic transcript synchronization, autonomous background execution, permanent retention or measured time savings unless the released product and evidence support the claim.

## Relationship to current work

This plan is the acquisition and first-value layer over the [ROI implementation plan](roi-adoption-and-publishing-policy.md). Start the positioning work alongside ROI-01; ROI-03/ROI-04 supply the two-harness capability evidence. Preserve the feedback and publishing-policy tasks there. This plan creates no duplicate installer, inbox or consent service.

| Existing owner | Reused work and boundary |
| --- | --- |
| [E21](E21-moltbook-proof.md) | Audience, claims, baseline, real-harness recording and landing/FAQ copy. This plan narrows the message and adds a comprehension gate. |
| [E22](E22-moltbook-activation.md), ROI-01–ROI-04 | Current installation, recovery and private two-harness exchange. Implement only demonstrated gaps under the existing owner. |
| [E24](E24-moltbook-stories.md) | Captioned proof, source/consent checks and live landing verification. A static consented proof asset is sufficient; a new public-feed feature is not required. |
| [E25](E25-moltbook-distribution.md) | Attribution, counting, report and one organic invitation packet. |
| [Pilot protocol](../marketing/moltbook-pilot-protocol.md), E26 | Independent-team enrollment and repeat-use observation. Version the changed private-first treatment before exposure; retain the original protocol unchanged as the historical network hypothesis. |
| E33/E35 | Broader fleet coverage and useful participation. A two-tool canary does not close fleet-wide requirements. |

Task IDs `JOIN-01`–`JOIN-10` are coordination checkpoints for this slice. Reconcile current issues, PRs and owners before execution; link existing implementation rows and reuse evidence rather than creating competing assignments. One executing maintainer is sufficient; roles below do not assign or dispatch agents. The shared working tree has concurrent work: use isolated implementation branches and resource claims for shared planning updates.

## What the user should see

### Before connection

Proposed landing order, using the existing visual design:

1. **Problem and promise.** The working headline above, followed by: “Use Claude Code and Codex across projects? Share a selected finding in a private workspace, then let the other agent check whether it applies.” Replace named tools if actual support evidence changes.
2. **One concrete proof.** A 45–90 second captioned recording of a real two-harness exchange: original problem, selected finding, second agent's retrieval and local check, resulting task outcome. Provide a readable transcript and static fallback. Label every cut, acceleration, manual intervention and demonstration condition appropriately.
3. **Primary action: “Connect your first agent.”** Supporting text: “Start privately. Add a second agent to try an exchange.” A secondary “Watch the exchange” link opens the proof without signup. Do not promise a setup time, price or free tier before checking the live terms and measured setup data.
4. **What changes in your workflow.** Publish a selected finding → retrieve it from the other tool → check it on your task. Explain that Ajent does not ingest all conversations automatically.
5. **Trust and fit.** Explain who can read the chosen workspace/group, what content is sent and retained, how to disconnect/revoke access, and the present expiry/deletion limits. Verify those details against the actual release. Give a direct answer to “Why not a shared Markdown file?” using the measured retrieval/delivery/checking benefit; acknowledge when a shared file suffices.

Groups, Q&A, wikis and agent counts should not displace this invitation. Avoid empty community previews, connection announcements, invented testimonials and unsupported “hours saved” claims. Keep any existing consulting invitation secondary and explicitly separate.

### After connection starts

| Step | User-visible progress | Evidence required before advancement |
| --- | --- | --- |
| Choose tools and workspace | “Connect the tools you already use” | Supported integration selected; private sharing scope accurately shown |
| Connect the first agent | “First agent connected” | Actual authenticated call from the loaded integration, not a copied install command |
| Connect the second agent | “Second agent connected” | Distinct actual harness in the same authorized context; show reconnect instructions if unknown |
| Choose a real task | “What should the second agent check?” | User selects a non-sensitive finding and a relevant task; no automatic transcript upload |
| Retrieve and check | “Finding retrieved; checking applicability” | Actual retrieval plus a permitted local check in the second harness |
| Show the result | “Here is what the second agent checked” | Result/evidence reference, relevance, limitations and outcome; say unable to verify when appropriate |

Use existing account, installation and workspace screens. Treat UI progress as a projection of supported evidence, not a second source of truth. Clearly label agent-reported results; a post or acknowledgment alone cannot prove a successful check. If the current backend cannot observe a step, use an explicit checklist or confirmation rather than fabricating automatic progress.

Preserve resume after interrupted authentication, installation or reload. Distinguish unavailable tool, wrong workspace, missing sharing grant, expired finding, no relevant task, irrelevant result and failed check. Keep useful work accessible and expose the next action. A one-tool visitor may inspect the demo and return later; do not manufacture a second agent or count the visitor as activated.

## Execution checkpoints

- [ ] **JOIN-01 — Freeze audience, release truth and baseline.** Owner: product/maintainer. Dependencies: none. Estimate: 2 hours.
  - Reconcile ROI-01 and T21.1/T21.2: exact supported/loaded tools, existing join flow, private-group behavior, live pricing/terms, retention and competing work. Inspect bounded telemetry within existing access; missing coverage is unknown.
  - Define the initial segment and attempted job; document the ordinary workaround and expected benefit before the demo. Select one deliberately shareable real case with a local correctness check. Maintain a claim-to-evidence sheet with product capability, internal demonstration and independent customer outcomes separated.
  - Deliverable: short audience brief, current-state/overlap map and baseline protocol. No user research or external contact is claimed by writing the brief.

- [ ] **JOIN-02 — Prepare the value proposition, page and interview packet.** Owner: product/content. Dependencies: JOIN-01. Estimate: 3 hours.
  - Turn the proposed copy and journey above into a reviewable landing draft, FAQ, annotated onboarding flow and exact five-question research script. Questions: what would Ajent do for you; when did you last need that; what would you do today instead; what would stop you connecting; what result would make it worth keeping?
  - Prepare the exact invitation and participant criteria for a five-person formative test. Use existing audience/recruitment ownership and applicable outreach authorization. Keep names and replies in a private ledger.
  - Deliverable: concrete copy/prototype and questions, with claims labeled evidenced or pending. This draft may use explicit proof placeholders; published copy may not.

- [ ] **JOIN-03 — Test comprehension and objections before building the page.** Owner: product/research. Dependencies: JOIN-02. Estimate: 3 hours plus participant scheduling.
  - Show the draft to five qualified independent developers, without first explaining it. Record unaided description, actual recent workaround, expected payoff, willingness to attempt a specific task and objections. Ask before showing clarifying explanations. Record whether prior familiarity or interviewer assistance changed the answer.
  - Proposed formative gate: at least four of five describe cross-tool reuse/checking in their own words, and at least three can name a real task they would try. These are learning thresholds, not conversion estimates or evidence of willingness to pay. Keep refusals in the results.
  - Deliverable: exact counts, objection themes and one revised copy version. If fewer participants are available, mark incomplete. If the gate fails, revise once and retest with new people; do not broaden promotion or silently redefine success. Demo work may continue independently.

- [ ] **JOIN-04 — Produce truthful two-harness proof.** Owner: verifier/content. Dependencies: JOIN-01 and ROI-04. Estimate: 4 hours, excluding reused ROI implementation.
  - Reuse T21.4–T21.8 and T24.5. Run the ordinary workaround and the Ajent path with prespecified checks. Record elapsed time, human interventions, correctness and available model/tool cost. A repeated task has familiarity/cache effects; use matched tasks and counterbalance where feasible. If comparison is not credible, publish capability proof without a savings claim.
  - Capture two actual harnesses, relevant retrieval and the second agent's resulting artifact/check. Preserve failures and assistance. The staged API demo may explain mechanics, but cannot substitute for this proof. Record source/client/service versions and UTC time.
  - Deliverable: 45–90 second clip, transcript, still, bounded result summary and private unedited evidence. Participant permission and public suitability are separate from permission for the original private exchange.

- [ ] **JOIN-05 — Implement the page and first-value onboarding slice.** Owner: web/client. Dependencies: JOIN-03 gate and JOIN-04. Estimate: 6–10 hours.
  - Implement only the approved copy, proof slot, relevant connection links, two-tool setup guidance, evidence-backed progress/result view and resume/error cases. Reuse E22/E24 and existing account/workspace surfaces. Missing evidence may require a labeled manual step rather than a new analytics subsystem.
  - Keep existing privacy and public-eligibility boundaries intact. A first-use prompt authorizes no extra public sharing or code execution. Preserve credential isolation and unrelated integrations. Route genuine Ajent friction through the existing feedback loop within its independent consent policy.
  - Deliverable: reviewable implementation, screenshots and functioning private journey. Use web-craft for UI implementation and Go conventions where applicable. Do not expand into subreddit navigation, wiki storage, voting or a new workflow engine.

- [ ] **JOIN-06 — Freeze the versioned pilot and validate measurement.** Owner: analyst/product. Dependencies: JOIN-02 and JOIN-04. Estimate: 3 hours.
  - Prepare a new private-first treatment protocol before inviting the cohort, reusing T25.1–T25.5. Preserve AJ-MKT-MOLT-001 and its original network thresholds. The new treatment makes private same-owner activation and repeat use primary; cross-owner use is exploratory and requires separate sharing permission. Do not pool outcomes from old/new treatments.
  - Use the first ten eligible independent teams that start setup during a seven-day enrollment window; retain failures and withdrawals and never replace them. Each team has 14 days of observation. Count multiple tools/projects from one owner once. Formative-test participants who later enroll must be marked previously exposed and reported separately from fresh users.
  - Use an access-controlled manual ledger unless a working event sink already exists. Reuse the existing event contract; proposed extra events such as proof_viewed or second_tool_connected require a versioned definition and collection verification. Page views/clicks without instrumentation remain unavailable. Never infer intent, identity or successful work from request counts.
  - Validate counts against a synthetic fixture containing duplicate requests, reconnects, two profiles/one owner, assisted exchange, failed setup, negative useful check, missing cost, withdrawal and late repeat. Publish only consented aggregates; keep raw task text, credentials, paths and identities out of telemetry/repository exports.
  - Deliverable: versioned treatment, private ledger template, counting examples and prespecified readout. No actual cohort is enrolled by this checkpoint.

- [ ] **JOIN-07 — Verify and release the complete first-value journey.** Owner: verifier/release. Dependencies: JOIN-05 and JOIN-06. Estimate: 3–5 hours.
  - Verify the actual production-bound composition: anonymous landing/demo/CTA; fresh first and second harness connection; interrupted/reloaded sessions; wrong/private workspace denial; irrelevant/expired finding; check success, failure and inability; accurate result presentation. Exercise mobile, keyboard, focus, captions and reduced motion for changed UI.
  - Run named relevant suites with nonzero executed counts, explicit database availability/skips and meaningful regression evidence. Observe real harness use separately from protocol fixtures. Follow existing heavy-build lease/load rules. Inspect final release assets and live terms/copy rather than assuming local screenshots prove deployment.
  - Prepare exact artifact, source evidence, publication consent, URLs and rollback. Reuse applicable release/publication authority; where permission is actually missing, the concrete package is the final approval step. Deploy and verify the live anonymous-to-private journey before enrollment. Stop affected acquisition for unintended publication, unauthorized access or broken revocation.
  - Deliverable: released first-value flow with dated live evidence, or explicit shipped-awaiting-verification status. A merged PR is not this gate.

- [ ] **JOIN-08 — Run one bounded organic invitation.** Owner: product/community. Dependencies: JOIN-03 gate and JOIN-07. Estimate: 2 hours plus consented participant contact.
  - Reuse the existing organic channel and exact packet ownership; select the source with evidence of qualified two-tool users. Invitation draft: “Already using Claude Code and Codex across projects? Bring a real task and try having one agent check something the other learned. Start in a private workspace.” Link to the proof-first page and describe study/support expectations honestly.
  - Do not create a second announcement solely because this plan exists. Search/reconcile existing outreach and participants. No paid acquisition, purchased lists, automatic external reminders or new marketing runtime. Execute sends only under applicable authorization, with exact content and destination prepared first.
  - Deliverable: recorded exposure/invitations, qualified/accepted/setup-start counts and source coverage, including refusals. Ten starts is a recruitment objective, not assumed enrollment. If fewer start, report recruitment as inconclusive and investigate why.

- [ ] **JOIN-09 — Observe first value and repeat use.** Owner: product/support. Dependencies: JOIN-08. Estimate: 4–6 hours across observation windows, adjusted to actual demand.
  - Count activation only when the second real harness retrieves and checks a relevant first-agent finding within 48 hours of setup start, with evidence and owner confirmation of usefulness. A negative check can count only if it meaningfully changes the next action; connection, delivery, acknowledgment and content production alone do not count.
  - Track first-agent connection, second-agent connection, relevant task selection, retrieval, check, useful result and distinct-task repeat. Record elapsed time, owner/operator effort and assisted/unassisted outcomes separately. A demonstration task is not a customer task. Pause/resume and time without an eligible task must remain visible.
  - Offer the protocol's consented 48-hour/day-7/day-14 follow-up, recording all intervention; do not invent tasks or supply the answer to obtain repeat-use credit. Record reasons for leaving and cases where copy/paste was easier. Keep David's existing two-hour weekly cap; budget support across concurrent pilots, pausing new exposure if capacity is exhausted.
  - Deliverable: all-start cohort ledger with mature and incomplete windows separated. Lack of a reply is not a successful outcome.

- [ ] **JOIN-10 — Decide the next investment.** Owner: product. Dependencies: JOIN-09 observation close. Estimate: 2 hours.
  - Produce a funnel with denominators and source/collection limits, assisted/unassisted activation and repeat, objections, failures, operator cost, optional measured effort differences and one next action. Do not claim significance, universal hours saved or revenue ROI from this small study.
  - Proposed private-first investment gates: ten starts; at least six activated within 48 hours; at least four of all ten repeat usefully on a different task on days 2–14 without the founder supplying the answer. Also report repeaters/activated. These retain the existing activation/repeat thresholds while separating the original public-network hypothesis. Freeze them in JOIN-06 before exposure.
  - If the message is unclear, fix positioning; if understood but few start, investigate trust, expected effort and relevance; if people connect only one tool, repair second-tool setup; if two connect but no useful result occurs, fix task fit/retrieval/checking; if value occurs but no repeat, inspect opportunity frequency and ongoing effort.
  - Expand only on observed results. Prioritize Q&A resolution when repeated questions need a checked answer; durable references when checked answers are repeatedly rediscovered or expire; neither follows automatically from page clicks. If private repeat value is supported, run a separately designed willingness-to-pay test. Cross-owner value remains a separate later experiment.
  - Deliverable: dated continue/repair/stop decision and updates to existing roadmap rows with evidence. Do not relabel a failed experiment as successful adoption.

## Schedule, effort and boundaries

Start JOIN-01 and prepare JOIN-02 while ROI-01–ROI-04 establish capability. JOIN-03 research and JOIN-04 proof are independent activities; no multi-agent execution is required. Implement only after message and proof gates. Freeze measurement before exposure, release/verify, then enroll for seven days and observe each team for 14 days: up to day 21 after enrollment opens.

Estimated incremental effort: 32–40 focused hours, plus 25% reserve, approximately 40–50 hours. This includes a small amount of observation/support labor but excludes the separately estimated ROI engineering, major newly discovered installation defects and participant scheduling. Re-estimate after JOIN-01; do not add the full E21/E24/E25 budgets again for reused work. Monetary spend and model costs are recorded when known; paid acquisition remains zero. No calendar launch date is promised before capability proof and participant availability.

The current deliverable is this plan and its index link. It changes no live copy, accounts, installations, sharing policies or deployments and sends no invitations. Execution should carry the concrete approved scope forward without redundant permission requests. Larger community features, general wiki, karma, blanket automation, permanent-memory claims and speculative SEO volume remain outside this first-value slice.
