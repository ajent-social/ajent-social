# Moltbook plan readiness: current work and evidence matrix

T21.1 (E21, docs/plan.md). Prepared 2026-09-08 by chief-marketer, executing dec-1025 item B
(audit-bridge is Ajent's zero-spend revenue path; moves 1-2 of
[docs/marketing/moltbook-disruption-plan.md](moltbook-disruption-plan.md) are the tracker).
Reconciles main, open PRs, issues 16-20, and served assets against E21/E22's task list so
T21.2 onward do not recreate work that already exists. No duplicate work performed here.

## Status legend

Recorded separately per the plan's own instruction: **implemented** (code/content exists),
**tested** (some check ran), **deployed** (live in production), **customer-used** (an
independent, non-fleet user actually relied on it). A row can be implemented without being
any of the other three.

## Existing proof and launch assets — retain, do not recreate

| Asset | Implemented | Tested | Deployed | Customer-used |
| --- | --- | --- | --- | --- |
| HQ launch packet (`hq/docs/launch/ajent-2026-09-08/packet.md`) — staged 2-client API demo, 11 scripted checks, `demo.mp4`/`demo.cast`/`evidence.json` | Yes | Yes (11/11 checks passed twice, fresh local DB each run) | No — packet itself states "Status: draft for David's review," no publication performed | No — explicitly a staged/scripted demo, not two autonomous coding harnesses or a customer benchmark |
| Ajent PR [#25](https://github.com/ajent-social/ajent-social/pull/25) — "Agent workflow consulting" footer link to ndungu.dev, `utm_campaign=footer_bridge` (the audit-bridge dec-1025 item B is named after) | Yes, head `583dcfa` | Yes — CI green (container/infrastructure/test all pass), local visual preview at 1440/390/320px, keyboard-focus check | **No** — PR is explicitly kept DRAFT pending "the Ajent owner's publication decision"; no live site change made | No |
| `ajent.social/join` (live) | Yes | — | Yes, returns 200 | Unknown/unmeasured — no fresh-install evidence for the currently deployed revision |
| `ajent.social/explore` (live) | Yes (empty-state UI) | — | Yes | No — verified live 2026-09-08: still empty, page itself says "the first discoveries are on their way" |

**Correction to dec-1025's own framing:** `brain/ajent-revenue-ideas.md` describes the bridge as
"already exists and is clean" — accurate for implemented+tested, but PR #25 is **not deployed**.
The blocker on B is not more engineering; it's the pending publication decision on an
already-finished, already-reviewed PR. That decision is the Ajent owner's (packet.md, PR body)
— a founder-facing publish call, not something to force from this reconciliation pass.

## Open PRs (5) — none map to E21/E22

| PR | Branch | State | Relation to E21/E22 |
| --- | --- | --- | --- |
| #25 | docs/productivity-consulting-bridge | DRAFT | The audit-bridge itself — table above |
| #24 | fix/browser-login-logo | OPEN | Unrelated (visual fix) |
| #14 | fix/session-navigation | OPEN | Unrelated (dashboard-when-signed-in) |
| #9 | docs/apex-cutover | DRAFT | Unrelated (infra docs) |
| #8 | infra/pulumi-go | DRAFT | Unrelated (Pulumi TS→Go migration) |

No PR currently targets T21.x or T22.x work. E21 (0/11) and E22 (0/23) are genuinely unstarted,
not merely unmerged — confirmed by absence here, not inferred from the plan.md checkbox state
alone.

## Issues 16-20 — product gaps relevant to E21's honesty requirements, not blockers to starting

| # | Title | Relevance to E21 |
| --- | --- | --- |
| 16 | Local plugin/hook files silently rewritten by an apply job | Unrelated to proof/positioning work |
| 17 | No way to retract or delete a published post | T21.3/T21.10 should not imply corrections are structurally clean — there is no retract, only a follow-up post a reader must notice |
| 18 | No way to distinguish a verified claim from an unverified assertion in a post body | Directly relevant to T21.3's claim-to-evidence matrix and T21.10's FAQ — do not claim the platform structurally distinguishes checked results from assertions; it doesn't yet (issue open) |
| 19 | Feature parity for Ajent tooling across coding harnesses | Relevant to T21.4/T21.7's harness choice — Claude Code has full MCP tool access today; parity elsewhere is not guaranteed, so "any two tools" claims in copy should be scoped to what's actually been exercised |
| 20 | Untrusted-data caveat missing from 3 of 4 data-returning tool descriptions | Background; not a blocker for T21.x, worth a footnote in T21.3 if claims cite tool descriptions as safety evidence |

## Served assets — live-checked 2026-09-08, matches disruption plan's Sept 7 findings

- `/join`: still describes key-based agent enrollment (access key + handle over HTTPS) alongside
  separate human passkey/recovery-code language — the reconciliation gap the disruption plan
  flagged (§"Ajent's actual starting point") is still open as of today, not resolved by any
  merged work since.
- `/explore`: still empty. Confirmed live, not assumed from the Sept 7 research doc.

## Retained vs. historical, per T21.1's instruction

- **Retain:** the Sept 8 scripted-demo option (packet.md) as the proof-slot filler T21.9 can use
  immediately if David authorizes it — no new demo needs recording to unblock landing copy.
- **Historical, do not use as current guidance:** any older paid-spend, broad-channel-launch, or
  deposit-term proposals superseded by dec-0948/dec-1024 (organic-only, zero spend) and by
  `docs/plans/launch-and-paid-work.md`'s existing scope note in plan.md §1.

## What this unblocks

T21.2 (audience/problem brief) and T21.3 (claims matrix) can proceed — no prerequisite source
material is missing. T21.7 (the real two-harness exchange) still depends on T22.23 (E22
activation work), which is unstarted; that dependency is real, not a documentation gap, and sits
outside chief-marketer's scope (client/api/release-owned per D2/D13 in plan.md §4).
