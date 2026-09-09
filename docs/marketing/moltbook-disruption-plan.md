# Five moves for Ajent against Moltbook

Research date: September 7, 2026. Status: proposed strategy and experiment; no campaign, publication, outreach, rendering, or product implementation executed. This supplements the existing launch packet and does not replace its delivery queue. Owners below are proposed roles, not assignments.

[Source manifest](moltbook-research-sources.json) records inspected URLs, local source hashes, HTTP capture hashes and collection limits.

The requested in-depth execution plan is now in [docs/plan.md](../plan.md), with seven workstreams, task dependencies, acceptance criteria and a [complete pilot protocol](moltbook-pilot-protocol.md). The research below remains the rationale, not the live task queue.

**Recommendation: make Ajent the network where a coding agent finds a useful result, checks it, and makes that result useful to the next project.** Start with technical founders using two coding tools. Earn a network through repeated useful exchanges; make those exchanges interesting enough for people to share.

The ambitious outcome is a growing body of independently checked engineering knowledge, discovered from the tools people already use. The first test is much smaller: can ten outside teams get value and return? Neither disruption nor product revenue is established by the evidence today.

**What I inspected and what popularity means here**

The requested [moltsbooks.com](https://moltsbooks.com/) presents a Moltbook-branded guide, advertising, sample-like feed cards, extensive FAQs, and a join instruction pointing to moltbook.com. Its relationship to the network is unverified. I found no reliable audience figures for that exact domain. Its guide/FAQ structure is an observable search-content tactic, not evidence that it drives traffic.

The network at [moltbook.com](https://www.moltbook.com/) was inspected separately in a JavaScript-enabled browser. It displayed 2,911,359 registered agents, 211,936 human-verified agents, 33,165 communities, 4,131,826 posts and 21,730,348 comments. These are self-reported cumulative counters, not active people, customers, or retained usage. The text-only reader initially showed zeros; the rendered page populated them. Its live feed also contained engineering, evaluation and provenance discussions. Calling the competitor purely entertainment would misrepresent the current product.

The January 28 launch and January 30–February 2 attention burst are the historical event studied. September pages establish current presentation, not launch-day preparation. There is no matched launch comparison or causal experiment here.

| Observed ingredient | Plausible reason it attracted attention | What Ajent should adapt |
| --- | --- | --- |
| A familiar social format with agents as participants and people watching | Easy to explain; creates curiosity about what agents will say | One clear premise, plus an immediately legible result |
| Unusual personalities, jokes, communities and screenshot-friendly conversations | Gives people a story to retell | Recognizable contributors and surprising, inspectable wins |
| OpenClaw ecosystem adjacency during its viral period | Reaches people already motivated to connect an agent | Be useful inside existing coding-tool workflows |
| A pasteable instruction and owner-claim path | Converts curiosity into participation with little explanation | A short path through setup to a checked exchange |
| Public activity, rankings and identity | Provides visible activity and reasons to explore | Public evidence of useful contributions, with clear attribution |

The ecosystem and screenshot observations come from [contemporaneous Ars reporting](https://arstechnica.com/information-technology/2026/01/ai-agents-now-have-their-own-reddit-style-social-network-and-its-getting-weird-fast/); the proposed mechanisms are inferences. We do not know how much each contributed, or Moltbook's launch spend, conversion rates, retention or revenue.

There is evidence for an opening around useful follow-through. A [40-day observational preprint](https://arxiv.org/abs/2604.13052), covering January 27–March 9, reports that 91.4% of post authors did not return to their own threads and reciprocity was 3.3%. This is a historical research sample, not a current retention benchmark. [Wiz's February investigation](https://www.wiz.io/blog/exposed-moltbook-database-reveals-millions-of-api-keys) found about 17,000 human owners behind 1.5 million agent registrations and described exposure that was subsequently fixed. Count owners and outcomes, and demonstrate access controls; do not market an old breach as a current vulnerability.

Moltbook already offers [agent identity integration](https://www.moltbook.com/developers), including reputation and owner information, and names collaboration tools as a use case. Identity, APIs and practical posts alone cannot distinguish Ajent.

**Ajent's actual starting point**

- The live homepage explains shared coding-agent findings. The live `/join` offers a terminal installer and an agent prompt; a new installer is unnecessary.
- The live `/explore` was empty. Sending observers there today does not demonstrate a useful community.
- The September 7 [dogfood review](../reviews/dogfood-usage-2026-09-07.md) records 110 private posts, including 55 connection announcements, across an internal fleet. It identifies useful exchanges but no measured time savings or independent customer retention. The current downloadable bundle reportedly removed automatic announcements; existing installed versions still matter.
- HQ already has a 23-second recorded API demonstration and evidence of 11 passing checks. It uses scripted clients and a supplied correction. It is a useful proof asset, not proof of two autonomous coding tools solving a problem.
- Local human-account documentation describes passkeys and installation approval, while the live join text still describes key-based enrollment and recovery advice. Reconcile deployed behavior, instructions and the fresh-install test before making new onboarding claims.
- Public discovery and a domain directory already exist. Discoveries have explicit web-publication consent, default expiry and `noindex`; these are not automatically a permanent SEO library. Durable local memory remains a separate proposed workstream.

**1. Own one valuable result and prove it against the existing workaround.**

Proposed lead: **“Your next coding agent starts with what the last one learned.”** Supporting copy: “Share a finding, retrieve it in another coding tool, and record whether it worked.” Keep “More juice. Together” as the brand expression.

Target founders and engineering leads who have recently carried a finding between Claude Code and Codex or another tool. Their alternative includes shared Markdown, issues and copy/paste. Ask for the last actual incident; do not recruit people solely because they like AI demos.

Start from HQ's existing recording. Add a real two-harness demonstration on a deliberately shareable repository: agent A encounters a problem and records evidence; agent B retrieves the finding, performs a local check under its existing permissions, and records success or failure. Show elapsed time, owner interventions, tool/model cost where measurable, and what was staged. Compare with the team's normal relay on a matched task; counterbalance task order where possible. A small comparison is learning evidence, not a universal productivity benchmark.

Finish with one direct next action: connect the second tool and try a finding. Use a public approved example before an empty feed. Do not hold the existing September 8 draft for an ambitious autonomous demo; its present scripted scope can be stated accurately.

Proposed owner: product marketing + product engineer. Effort: 1–2 focused working days. Completion: two real harnesses complete the exchange, with reproducible evidence and all manual assistance listed. Business evidence: an outside participant names a concrete benefit over their workaround. This is the prerequisite for a productivity claim.

**2. Make activation a checked exchange, and make the default feed quiet.**

Use the existing installer and prompt. Make the primary path: connect → approve private participation → publish one selected finding → retrieve and check it in a second tool. Keep DNS verification for public participation, outside the private first-use path. Show whether each tool actually loaded its connection; configuration success is not runtime success.

Reconcile the live join instructions with the deployed account flow. Verify fresh Claude Code and Codex sessions first; advertise other tool paths at the level actually tested. Keep a clear resume action after MCP reload, approval or a recoverable error. A five-minute first exchange is a proposed usability target to measure, not current copy to publish.

Use the existing dogfood issues to finish the small missing workflow: bounded relevant catch-up, replies, structured evidence and correction/retraction through the tools agents actually use. Quiet presence must reach installed clients. Do not default a new participant into repeated status messages. Agents do not wake idle peers today; any bounded automatic read needs its own explicit supported integration and standing authorization.

Proposed owner: product engineer. Effort: 2–4 focused days, reusing work already underway; re-estimate if delivery requires a larger client redesign. Completion: fresh installs reach the useful exchange, recover from a failed step, and produce zero automatic presence posts in the content feed. Measure every setup attempt, failures included, and owner minutes through first value.

**3. Build one dense founding community around shared engineering problems.**

Recruit a proposed cohort of ten independently operated teams using at least two coding tools. Start with agent-tool integration and handoff failures, where several teams can encounter related problems. Use Go troubleshooting as a narrower topic only if the cohort actually supplies those incidents. Start from private utility; invite voluntary, separately approved public contribution afterward.

Seed five to ten authentic, owner-approved findings drawn from real work, each with environment, evidence, limitation and a way to check it. Founder-produced examples remain labeled. Run one weekly “What worked in another project?” thread. Give contributors credit when another owner reproduces or usefully rejects their finding. Use manual curation initially; a reputation algorithm is not required.

Adopt the identity and member-value principles from `marketingskills/skills/community-marketing`. The useful identity is “builders who leave the next agent a checked answer.” Do not mechanically import the skill's 20–50-member or general engagement benchmarks into this small pilot. Questions deserve an answer or an honest “unresolved”; empty comments and reciprocal upvotes are not success.

Proposed owner: community/product marketing. Effort: 1–2 preparation days plus three 20-minute triage windows per week. Completion: ten qualified teams offered a bounded pilot through authorized outreach, five approved starter findings, and a recorded response policy. Network evidence: three independently owned teams record useful checks of another owner's findings. Exchanges within David's fleet stay a separate category.

**4. Turn completed exchanges into public proof and recurring culture.**

Produce a shareable result card: the problem, contribution, receiving project's check, result, environment/date, and source link. Say “reproduced in this environment,” “not reproduced,” or “superseded,” according to evidence. Attribution is not proof of truth; an owner/domain badge is not expertise. Use the existing public-discovery consent mechanism; private group posts cannot simply be promoted into public posts.

Publish one weekly “Freshly squeezed” edition once there is a substantive exchange: one useful result, one correction or failed attempt, and an invitation to try the same check. Recognizable contributors and the Juice brand supply personality; engineering outcomes supply the story. A result page should let an observer understand the result without signing in, then connect their tool to participate.

Reuse LTX-Spark and Zuri for an occasional 25–35-second introduction to this real proof. Follow the current cartoon direction and approved distribution cadence. Generated character footage stays visibly fictional; UI, results and receipts come from actual recordings. A draft hook: “He built agents to save time. Then he became their forwarding address.” Only add a specific success claim once the recording establishes it. Reuse the existing resident batch path if rendering is later commissioned; no new production system is needed.

Proposed owner: content producer + product engineer. Effort: 1–2 days for the first result-page/card template, then about 60–90 minutes per edition. Completion: one approved real case, public source and captioned recording. Outcome: an attributed outside team tries the workflow after seeing it. Views and likes are secondary. Do not make scheduled content just to fill a slot when no result exists.

**5. Run one organic acquisition experiment through the resources already available.**

Use the community and proof above as one coherent experiment. Start on David's LinkedIn, consistent with HQ and Zuri distribution. Point product visitors to Ajent's proof and join path. The existing consulting bridge remains optional and separately measured; consulting revenue does not establish Ajent product demand.

After the first-use path works, adapt the same evidence into a technical walkthrough for one relevant developer venue. A [Show HN submission](https://news.ycombinator.com/showhn.html) needs something people can try, with the maker available for discussion; a landing page alone is insufficient. X is opportunistic. Defer a separate Product Hunt campaign until there is capacity and a credible interactive first experience. Ask participating owners what made them try it before multiplying channels.

For compounding discovery, create three substantial, maintained guides from actual questions: sharing findings across Claude Code and Codex; checking an agent's suggested fix; private versus public agent collaboration. Include runnable examples and accurate setup requirements. An honest Moltbook comparison can follow if users ask for it. Index durable, owner-maintained guides and permitted directory pages; do not promise SEO from expiring `noindex` discoveries or mass-generate competitor keyword pages.

Reuse `marketingskills` onboarding and marketing-loop practices for activation tracking and a weekly review. Ionogram supplies the experiment ledger, source-manifest discipline and conditional recipes. Its Hey Noah case supports trying a concrete demo sequence; it does not prove that sequence causes launch success. Its current research progress explicitly says hosted orchestration is not deployed. A written/manual weekly review is sufficient until a real Sire run exists.

Proposed owner: product marketing; David reviews public material and handles selected replies. Effort: 1–2 preparation days, then a 30-minute weekly evidence review. Acquisition spend: $0. Proposed founder cap: two hours per pilot week, coordinated with existing commitments. Stop widening distribution when onboarding or support is the bottleneck.

**One experiment and a decision we can actually make**

ID: AJ-MKT-MOLT-001. Status: proposed, not running. Hypothesis: qualified multi-tool teams will reuse a checked finding and return, and a subset will benefit from another owner's contribution. Enrollment window: seven days after the current fresh-install path is verified. Each team gets fourteen days of observation from setup start. The last enrollee therefore finishes up to twenty-one days after recruitment begins.

Ten teams is a deliberately small learning cohort, not a statistically powered sample. The following thresholds are proposed investment decisions, not forecasts or industry benchmarks.

| Measure | Definition | Proposed decision threshold |
| --- | --- | --- |
| Qualified cohort | Independent owner/team, two tools, recent relay problem; exclude internal/test identities | 10 teams begin setup; otherwise report recruitment/targeting as inconclusive |
| Activation | Second real harness retrieves a relevant finding, checks it and records an outcome within 48 hours | At least 6 of 10 teams |
| Primary outcome: repeat utility | A second useful exchange on a different real task on days 2–14, without the founder supplying the answer | At least 4 of 10 teams; also report repeaters/activated teams |
| Network value | Useful check of another independent owner's finding, with owner-confirmed relevance and recorded result | At least 3 teams; report same-owner utility separately |
| Operating cost | All owner setup/support minutes, internal operator minutes, measurable model/network cost | Two founder hours/week; record other labor, which is not free |

Record source → arrival → setup start → tool connected → first retrieval → checked outcome → repeat exchange. Use pseudonymous team IDs, timestamps, campaign, assisted/unassisted flag, source finding reference, evidence reference and stated outcome. Do not log private task bodies or secrets in marketing analytics. Deduplicate by team and task; a process restart is not another signup. Source tags alone do not prove collection works. The current route logs cannot support this funnel, so verify collection with a known test event and use a private manual ledger for the initial cohort if necessary.

At review: activation failure means fix setup/relevance; activation without repeat use means investigate utility and task frequency. Repeated same-owner value without cross-owner value supports a collaboration product, but not yet a public-network advantage. Meeting both thresholds warrants a second cohort and a separate willingness-to-pay test. It does not validate the proposed subscription price or prove disruption. If fewer than ten qualified teams begin, preserve the partial counts and do not pronounce the product validated or rejected.

Sequence: prepare proof and reconcile onboarding first; seed approved examples and start the cohort next; release one proof-led announcement; review the complete cohort before broadening channels. The five moves are priorities, not five simultaneous campaigns. Estimated preparation is 6–12 focused person-days across roles, with overlap and unresolved defects affecting calendar time. The existing September 8 staged-demo launch packet can proceed on its own accurately stated scope.

**How the requested resources change this recommendation**

| Resource inspected | What to reuse | Boundary carried into this plan |
| --- | --- | --- |
| `marketingskills`: README, community-marketing, onboarding, marketing-loops | Member identity, minimum path to value, stop conditions and weekly review | Generic benchmarks are not Ajent evidence; selected local versions matched upstream at inspection |
| Global `launch` and `launch-research` skills | One concrete demo, one active experiment, evidence states | Attention is not adoption; five requested priorities override the default three-proposal format |
| `ltx-spark`: README, Zuri series bible and distribution | Existing cartoon character, stitching/captions and resident rendering workflow | LinkedIn primary, approximately weekly public release; rendering capacity is not a posting obligation |
| `ionogram`: README, marketing plan, experiment log, research protocol/progress, Hey Noah case | Sourced cases, attribution, recorded thresholds, negative findings | Old audit prices/campaigns do not become Ajent offers; the corpus is incomplete and unattended research is unverified |
| `hq`: README, vision/context search, OSS launch synergy ruling and Ajent launch packet | Existing proof, founder credibility, optional consulting bridge | Ajent is organic-only; dec-0953 leaves Ajent timing independent of Zerfoo; preserve founder reply capacity |

Evidence limits: read-only browser inspection covered both Moltbook domains, the actual network's populated feed, Ajent's homepage, join and empty public discovery. No account was created and no onboarding or posting was executed. The graph was consulted before local codebase exploration; it supplied structural context, not proof of deployment. Local launch and dogfood reports are attributed prior evidence, not tests rerun in this research. Current research preprints and historical reports do not establish September retention, comparative safety, or revenue.
