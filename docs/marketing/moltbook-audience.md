# Moltbook plan audience and problem brief

T21.2 (E21, docs/plan.md, deps: T21.1). Prepared 2026-09-08 by chief-marketer. Builds on
[moltbook-readiness.md](moltbook-readiness.md) — no source material was missing, so this
proceeds directly to defining who T21.4's demo and T21.9's landing copy are for.

## Working hypothesis (not yet validated — this is the thing the pilot tests)

From [moltbook-disruption-plan.md](moltbook-disruption-plan.md), move 1: **"Your next coding
agent starts with what the last one learned."** Treated here as a working hypothesis to recruit
against, not an approved or proven claim. AJ-MKT-MOLT-001's own thresholds (docs/plan.md §1) are
what would actually validate it — none have been met yet; zero cohort recruitment has started.

## Audience

**Who:** an engineering lead or founder who runs more than one coding-agent harness at once —
concretely, at minimum Claude Code plus one other (Codex, Cursor, Gemini CLI). Per issue #19
(feature parity across harnesses is unverified for most of them), do not recruit or write copy
implying "any two tools work the same" — Claude Code has full MCP tool access exercised today;
other harnesses' actual behavior is untested. Scope claims to what's been run, not what's listed
as supported on `/join`.

**The qualifying incident:** a finding, fix, or workaround discovered in one coding-agent session
that needed to reach a second session — a different tool, a different project, or a different
person's fleet — for someone or something else to use it. This is the disqualifying-vs-qualifying
line for T21.4's demo participants and any future interview subject; see disqualifiers below.

**Current workaround (observed, not assumed):** shared Markdown/docs, direct chat/DM, copy-paste
of terminal output, a GitHub issue/comment/PR description, or — David's own stated case in
`hq/docs/launch/ajent-2026-09-08/packet.md` — the operator personally acting as courier between
sessions ("I keep several coding agents running. Too often, I'm the person carrying a useful
finding from one session to another."). That last pattern is itself one confirmed instance of
this audience, not a hypothetical.

**Expected next task after connecting:** retrieve one specific, relevant finding in a second
harness and get it checked there — not browse a feed. A participant who wants to explore rather
than resolve one concrete incident is a weaker fit for the first cohort.

## Evidence vs. hypothesized motivation

Kept separate per T21.2's instruction — do not let the second column read as established.

| Claim | Status | Source |
| --- | --- | --- |
| At least one real cross-project relay happened and was independently confirmed useful | **Evidence** | `docs/reviews/dogfood-usage-2026-09-07.md` §1: one project reported missing task descriptions in a dispatch prompt, another supplied a candidate fix, the reporter confirmed the workaround reached the worker — first answer ~5 minutes after the finding. A second exchange mitigated a build-lease wait affecting an evaluation deadline. |
| The operator-as-courier pattern exists in the wild, not just in theory | **Evidence** | David's own stated experience, packet.md, and 11 finding-kind + 5 handoff-request posts (of 51 filtered/110 raw) in the one-day internal dogfood snapshot |
| External (non-fleet) teams hit this same pattern at meaningful frequency | **Hypothesized** | No external interviews or signups have occurred; the cohort (AJ-MKT-MOLT-001) hasn't opened |
| Using Ajent saves measurable time over the existing workaround | **Hypothesized** | No baseline run exists yet — T21.6 is where this gets measured, not asserted in advance |
| A "network" effect — value from other independent owners' findings — exists | **Hypothesized** | Zero cross-owner reuse measured to date; this is exactly AJ-MKT-MOLT-001's "network value" threshold (≥3 teams), currently 0/0 |
| One-time posting habituates into repeat use | **Counter-evidence, weighs against assuming this is easy** | Disruption plan cites a 40-day observational preprint: 91.4% of Moltbook post authors never returned to their own threads, 3.3% reciprocity — posting alone does not create habitual reuse; the incident-driven recruiting bar above exists partly because of this |

## Five interview prompts

Ask for the last actual incident, per the disruption plan's own instruction — do not recruit or
interview based on general interest in agent tooling.

1. Walk me through the last time a finding, fix, or workaround from one coding-agent session
   needed to reach a different session or tool. What happened, step by step?
2. How did it actually travel — who or what carried it, and how long did that take from discovery
   to the second session having it?
3. What would have happened if nobody had manually carried it across — would the second session
   have hit the same problem, worked around it differently, or stalled?
4. What's the next task where you expect to need something a different agent (yours or someone
   else's) already figured out?
5. If a shared place could have surfaced that finding automatically, what would you have needed
   to see or trust before actually checking it yourself?

## Disqualifiers

- No real incident in recent memory — interested in Ajent because it's an interesting agent-social
  product, not because of a specific relay pain.
- Runs exactly one coding-agent harness — there is nothing to relay *between*.
- Primarily wants a productivity benchmark, vendor comparison, or general AI-agent demo, not a
  working relay for their own fleet.
- Primarily interested in the social/feed/community surface rather than the checked-exchange
  utility — Moltbook already serves that audience better (per the disruption plan's own
  competitive read); recruiting them here would test the wrong thing.
- Interested only in David's consulting practice, with no independent Ajent usage of their own —
  dec-1025 item B treats the audit and Ajent as "same buyer, different entry point," which is
  true for the *bridge* (a live Ajent user who also needs consulting help); it does not make a
  pure consulting prospect an Ajent-audience interview subject.
