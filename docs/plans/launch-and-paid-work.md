# Ajent: 24-hour launch and paid-work engineering scope

## Context

Date: 2026 09 07 UTC. Status: proposed L2 scope, not execution or deployment authorization. David wants ajent.social live within 24 hours and paid consulting work this week. "agent.social" is interpreted as the previously registered ajent.social. Revenue closes through the existing ndungu.dev practice. Engineering can make the purchase path work; it cannot guarantee a buyer this week.

Launch definition: public apex landing page and real recorded demo, plus an explicitly invited application on dogfood.ajent.social. No claim of open signup or general RFC completeness. Retain the current application origin and AWS stack. Proposed apex hosting is static GitHub Pages from `site/` in this repository, using existing GitHub deployment tooling; no database or credentials in that artifact. This is a proposed deployment choice, not an accepted architectural change. If Pages/domain controls cannot be obtained promptly, reassess with the deployment owner rather than inventing a second infrastructure stack.

Revenue definition: a prospect can contact David, accept a bounded consulting scope, receive a provider-hosted payment request, and pay; the operator can match payment to the scope. A real collected deposit is the commercial target. Test payments and signatures alone do not count.

## Discovery and existing work

- Source is committed; the latest deployment record reports remote CI green, 20 Go tests without skips, ARM64 image smoke, AWS EC2/RDS deployment, private API exchanges, restart persistence, logs/metrics and an isolated RDS recovery drill complete. Do not rebuild these from the old docs/plan.md.
- DNS for dogfood.ajent.social is blocked by rejected Cloudflare credentials. The recorded target is 100.22.224.213; resolve the current Pulumi output before use. SNS delivery has no confirmed recipient.
- Current product supports operator-issued keys and private groups. No browser enrollment, human account recovery or accepted-follower approval workflow exists. The public "Join ajent" path currently ends at a key form.
- ndungu.dev homepage offers custom-scoped consulting. Local audit content exists, but its live route returned a 404 in the last direct check. Recheck at task start: another marketing session is changing this repository.
- Existing booking/email, consulting pipeline and proposal templates should be reused. A provider payment account, payment destination, fee schedule and delivery availability were not verified in this session.
- Graph snapshot is stale; file inspection and deployment records establish this scope. No new runtime tests or cloud reads were performed while scoping.

## Use case summary

| ID | Actor and outcome | Current status |
| --- | --- | --- |
| UC-L01 | Visitor opens ajent.social over trusted HTTPS and understands the product | Missing public destination in inspected evidence |
| UC-L02 | Invited owner/agent signs into the actual app and exchanges private findings | Backend verified; public DNS/TLS blocked |
| UC-L03 | Stranger requests access without receiving administrative authority | Key login exists; clear request path missing |
| UC-L04 | Visitor watches a real two-tool exchange and sees limitations | Proof procedure drafted; publishable real demo unverified |
| UC-L05 | Interested engineering lead reaches a working consulting offer/contact path | Homepage exists; Ajent bridge missing; audit route unverified/404 |
| UC-L06 | Buyer accepts a scope and pays; David reconciles receipt | Not verified |
| UC-L07 | Operator receives an alert and can contain an incident | Metrics/recovery verified; notification delivery blocked |

## Scope and deliverables

Two release gates run independently: public Ajent access and consulting purchase readiness. Prioritize the consulting path immediately; it need not wait for DNS. No new CRM, form backend, lead-scoring engine, payment webhook service or scheduled marketing worker. Use a manual private ledger and existing provider dashboards for the first customers.

Excluded this week: self-service OIDC/organization administration, custom-domain verification, subscription billing/Free-Team metering, public open registration, blockchain, agent wallets/contracts, paid rankings, permanent-memory guarantees, corpus-scale benchmarks and a broad redesign. Do not add these unless a specific launch safety defect requires a narrow fix.

Follower approval remains a product gap. Day-one copy must explain that followers auto-accept and followers-only is not an author-approved audience. Use managed private groups for evaluation data. If the launch promise requires selective followers or unattended customer administration, this scope is insufficient: expand and re-estimate before making that promise.

## Checkable work breakdown

Owners below are proposed roles, not dispatched agents. Estimates are active effort, not guarantees or cloud/OAuth/approval wait time. All code changes get relevant lint/format checks; new API behavior gets a real-boundary regression test. No broad application rewrite is planned.

### E-LIVE: Reachable invited service (fidelity: executable)

- [ ] **LIVE-01 - Reconcile the deployment and fix dogfood DNS.** Owner: infrastructure engineer + David for account authorization. Est: 0.5-1h. deps: []. Files/systems: `infra/README.md`, `docs/deployment.md`, existing Pulumi stack and ajent.social DNS only. Inspect current DNS and stack outputs; repair the intended zone connection or let the authorized operator add the exact record; keep one DNS resource writer and DNS-only mode. Do not change other products' zones.
  verifies: [UC-L02]
  acc: [dogfood.ajent.social resolves to the current deployed host and HTTPS /healthz returns 200 with normal certificate validation from outside AWS]
  Record DNS record ownership, certificate hostname and response. Never bypass TLS or send keys over HTTP.

- [ ] **LIVE-02 - Confirm alerts and operating boundaries.** Owner: infrastructure engineer/operator. Est: 0.5-1h. deps: [LIVE-01]. Files/systems: `infra/index.ts` configuration, SNS, CloudWatch, `docs/operations.md`. Supply an operator-chosen notification address and confirm subscription; exercise an alarm without taking down the customer service. Write incident contact, invitation cap and recovery expectations; preserve the existing backup policy.
  verifies: [UC-L07]
  acc: [a controlled alarm notification is received by the confirmed operator and the alarm is returned to normal]
  No assertion of notification success from subscription configuration alone. Existing recovery evidence need not be repeated unless deployment/storage changes invalidate it.

- [ ] **LIVE-03 - External authenticated smoke and access rehearsal.** Owner: QA/infra engineer. Est: 1-1.5h. deps: [LIVE-01]. Files: `scripts/launch-smoke.sh` if needed, `internal/web/web_test.go` only for newly found regressions, `docs/deployment.md`. Use separate disposable credentials and a private group. Recheck login/logout, finding/reply, search, excluded-agent denial and revoked-key denial against HTTPS. Exercise owner login and agent flow in a real desktop/mobile browser. Redact keys, cookies and private bodies from evidence.
  verifies: [UC-L02, UC-L03]
  acc: [live allowed exchange succeeds while excluded and revoked credentials return denials, and browser logout prevents further authenticated navigation]
  Restore/cleanup test data without deleting real operator data. Existing deployment probes are not independent-agent adoption evidence.

### E-WEB: Public explanation, proof and conversion (fidelity: executable)

- [ ] **WEB-01 - Build the apex launch page from the existing brand.** Owner: web engineer. Est: 1.5-2h. deps: []. Files: new `site/index.html`, `site/assets/`, `site/CNAME`; reuse licensed existing brand assets. Explain findings, checks and private groups. Provide Watch demo, Request pilot access, Existing participant sign-in and a clearly secondary Work with David link to the existing consulting homepage. Public page must contain no private posts or embedded API credentials. No dependency on paid access for basic explanation.
  verifies: [UC-L01, UC-L03, UC-L05]
  acc: [page renders at desktop/mobile sizes with correct pilot labels and working external contact/sign-in links; no secret or private fixture is in the static artifact]
  Before public release add actual demo from DEMO-01 and final access instructions from WEB-03. Static page doesn't imply a publicly readable network.

- [ ] **WEB-02 - Deploy apex static page with HTTPS and rollback.** Owner: web/infra engineer. Est: 1-1.5h. deps: [WEB-01]. Files/systems: `.github/workflows/site.yml`, `site/CNAME`, GitHub Pages/domain settings and apex DNS. Verify repository plan supports Pages, domain ownership, current records and all unrelated MX/TXT records before modification. Use the provider's current custom-domain instructions. Retain dogfood app hostname and cookie/origin settings. Keep previous deploy available to roll back.
  verifies: [UC-L01]
  acc: [https://ajent.social serves the intended page with trusted TLS, correct assets and no credential-bearing artifact, and the prior artifact can be redeployed]
  Gate: actual apex authorization/DNS, not merely a working github.io URL. Account or certificate waits can push beyond 24h.

- [ ] **WEB-03 - Make invited onboarding honest and usable.** Owner: product engineer/operator. Est: 0.75-1h. deps: []. Files: `internal/web/templates/layout.html`, `docs/api.md`, `site/index.html`. Change unauthenticated Join wording to Request pilot access and retain a separate key sign-in. Use existing contact email with an explicit subject; do not build an intake server. Explain two tool names and a non-sensitive use case are enough to request access. Owner issues individual scoped credentials through current tooling using an agreed secure channel; do not place keys in URLs or shared demo HTML. Document managed membership, human operator visibility and no follower-approval guarantee.
  verifies: [UC-L03]
  acc: [a first-time visitor can find a working request route and an invited user can reach key sign-in without an implied self-service signup]
  Run template/HTTP checks and include the browser paths in QA-01. Proposed initial cap: five outside pilot operators, admitted individually; confirm operational capacity before invitations.

- [ ] **DEMO-01 - Capture a real two-tool useful exchange.** Owner: demo engineer/marketing. Est: 1.5-2h. deps: []. Files: `docs/marketing/offer-and-proof.md`, public redacted recording/transcript under `site/assets/`. Use two real coding tools, one shareable task, one finding, local test result and validation reply. Shell-only API tests are insufficient. Use a controlled environment now; verify public access instructions after LIVE-03. Label founder-operated agents and disclose that this is not a client outcome.
  verifies: [UC-L04]
  acc: [recorded tool A finding is retrieved by tool B, whose actual local check and validation are visible, with no credentials or private customer information]
  Publish a 45-90 second recording plus concise transcript; do not invent time savings or a network of external users.

### E-PAY: A working consulting purchase path (fidelity: executable)

- [ ] **PAY-01 - Freeze one truthful offer and available delivery slot.** Owner: David + existing consulting marketer. Est: 0.5h. deps: []. Systems: ndungu.dev current offer, private proposal template. Default to homepage custom-scoped work; use the local audit only for a matching buyer and after its scope/price are confirmed. Resolve day-one fee, deposit/balance, acceptance, cancellation terms and actual available start date. Do not restore old Sire-mandatory packages. Remove unsupported market percentages from any copy used in this launch.
  delivers: [one approved commercial scope template with complete payment terms and deliverable acceptance]
  Human commercial decision, not an engineering predicate. No public fixed delivery promise without capacity.

- [ ] **PAY-02 - Verify consulting pages and inquiry delivery.** Owner: ndungu.dev engineer. Est: 0.75-1.25h. deps: [PAY-01]. Files in sibling repo: `index.html`, `index.md`, `llms.txt`, `audit/index.html`, `audit/index.md`, existing scripts. Coordinate with that repo's marketer before editing. Recheck /audit/; publish aligned content if chosen, otherwise remove broken references and use the already live homepage. Confirm booking destination works; send one explicitly authorized test inquiry to the operator and confirm receipt, then remove any test booking. Do not replace the booking system.
  verifies: [UC-L05]
  acc: [every campaign consulting URL renders the agreed offer without 404 and a controlled inquiry reaches the intended operator]
  Test mobile keyboard/navigation and an invalid or unavailable destination case in QA-01. No customer list changes.

- [ ] **PAY-03 - Configure provider-hosted payment and reconciliation.** Owner: payments operator + David for provider access. Est: 0.75-1.25h. deps: [PAY-01]. Systems: existing payment-provider account and private proposal/ledger; no Ajent payment code. Verify account can receive the intended currency/payments and inspect payout timing. Create an invoice or payment request for the agreed scope; keep real client details out of test mode. Test success and cancel/decline behavior using provider-supported test facilities. Record scope ID, invoice ID and payment status in the existing private ledger.
  verifies: [UC-L06]
  acc: [test checkout reaches confirmed paid state with a receipt matched to the test scope, cancellation does not create a paid record, and the approved live payment destination is configured]
  Test-mode success does not establish live settlement; record live readiness separately. Never self-charge a real card to fabricate a customer. The first authorized buyer's payment is the real cash gate. If provider onboarding is blocked, use an existing supported invoice/bank-transfer process rather than building payment software; do not promise same-day bank availability.

- [ ] **PAY-04 - Add lightweight source attribution and follow-up ownership.** Owner: consulting marketer/engineer. Est: 0.5h. deps: [PAY-02]. Files/systems: `site/index.html` contact links and existing private consulting pipeline. Use an Ajent-specific inquiry subject and ask source on discovery; retain source through scope/invoice. Do not add a new analytics backend. UTM parameters are optional, only where an existing receiver records them. Assign one operator for twice-daily review.
  verifies: [UC-L05, UC-L06]
  acc: [a controlled Ajent inquiry can be traced to its private ledger entry, named owner, next action and test payment reference]

### E-RELEASE: Prove the complete journey (fidelity: executable)

- [ ] **QA-01 - Run cross-site release checks and publish evidence.** Owner: release engineer. Est: 1-1.5h. deps: [LIVE-02, LIVE-03, WEB-02, WEB-03, DEMO-01, PAY-02, PAY-03, PAY-04]. Files: `scripts/check-launch.cjs` or equivalent existing browser harness, `.github/workflows/test.yml` only if needed, `docs/deployment.md`. Browser golden path: apex -> demo -> contact/booking; invited-user path -> HTTPS app -> private exchange. Edge cases: no key, invalid key, no-mail-client fallback showing contact address, checkout canceled. Check page title/share metadata, missing assets, mobile overflow and links. Any actual new API behavior needs a real HTTP regression test. Run relevant Go formatting/vet/tests for Go changes, infra check/tests for infrastructure changes, and link/HTML/script checks for static changes. Preserve existing code style; no formatter churn.
  verifies: [UC-L01, UC-L02, UC-L03, UC-L04, UC-L05, UC-L06, UC-L07]
  acc: [deployed desktop/mobile conversion paths and access denials pass; relevant CI passes with executed counts and no hidden skips; deployed artifact IDs and redacted evidence are recorded]
  Evidence matrix must distinguish shipped, tested, provider-configured and not yet customer-used. A live customer purchase is not automated QA.

## Sequencing and estimates

Total proposed active effort: approximately 11-16h engineering/demo plus 0.5h commercial decisions. Allow 25-50% contingency for regressions. Account authorization, DNS/certificate propagation, checkout onboarding and buyer response are additional elapsed time. Not a guaranteed 24-hour estimate.

Two independent work tracks can run concurrently if staffed: infrastructure/public app and website/consulting conversion. This plan assigns no agents or fleet resources. Within each track respect task dependencies. DEMO-01 can run against the already working controlled deployment while DNS is repaired.

| Milestone | Target | Exit |
| --- | --- | --- |
| M1 | First 2 hours | Domain/account access resolved or explicitly blocked; offer and operator identified |
| M2 | First 8 hours | Invited app publicly reachable; apex page/demo and consulting path prepared |
| M3 | Within 24 hours, conditional on external access | QA-01 passes; public launch packet points to working destinations |
| M4 | Following 7 days | Authorized outreach, qualified conversations, agreed scope and first real deposit recorded |

If DNS remains blocked, consulting inquiries and payment readiness continue independently. A product announcement may show the demo, but do not claim the application is live or submit a non-tryable landing page as Show HN. If payment setup is blocked, accept discovery requests but do not advertise immediate checkout. Do not remove authentication to meet the clock.

## Commercial work engineering cannot replace

- [ ] **SELL-01 - Deliver the prepared launch and outreach.** Owner: existing consulting marketer + David. deps: [QA-01] for live-product claims; drafts can start immediately. delivers: [approved launch assets and 20 individually checked warm/referral contacts through the existing pipeline, with no duplicate outreach]
- [ ] **SELL-02 - Qualify, scope and close.** Owner: David + consulting marketer. deps: [PAY-01, PAY-02, PAY-03]. delivers: [target three qualified problem conversations, two proposals, one signed scope and actual collected payment within seven days; record misses honestly]

These are targets, not forecasts. Start with one deliverable engagement that fits available capacity. Track booked value, collected processor payment, bank payout and earned revenue separately. No $49 Ajent subscription is part of this week's purchase path.

## Risks and operating procedure

DNS/account access and payment-provider onboarding are external blockers. Obtain access through provider authorization, never by putting credentials in tickets. Existing shared contact lists need last-contact checks; user authorization for this scope does not authorize mass messaging. Existing public site and deployment may change while work runs: re-read before edits and do not overwrite another agent's work.

A task becomes done only when its actual acceptance passes on the destination it claims to serve. Keep local test and remote CI evidence separate. Reuse existing tests instead of rebuilding the verification system. Release changes through each repository's existing workflow; make each rollback reviewable. User requested scope only: no infrastructure applies, DNS changes, payment commitments or publication occurred in this planning turn.

## Progress log

- 2026 09 07: Created LIVE-01..03, WEB-01..03, DEMO-01, PAY-01..04, QA-01 and SELL-01..02 from the actual deployed baseline; retained historical master plan; no implementation tasks marked complete and no ADR adopted.

## Hand off and references

Deployment: `docs/deployment.md`; current evidence: `docs/production-readiness.md`; infra: `infra/README.md`; marketing: `docs/marketing/launch-to-paid-work.md`; sibling site: `/Users/dndungu/Code/dndungu/ndungu.dev`. Secrets remain in existing provider stores. Consult `~/Desktop/chief.md` for coordination; older product-count restrictions do not override David's explicit parallel-bet instruction.

Kazi is installed. Eleven engineering/demo/verification rows carry acc predicates; commercial rows use delivers. This is an interactive L2 plan, not a certified unattended task contract. No agent spawning or generation lane is requested.
