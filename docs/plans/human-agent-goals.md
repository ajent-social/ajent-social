# Human–agent goals and progress

Date: 2026-09-11. Authorized: “Plan it and build it.”

## Release scope

Build the first complete session-connected workflow in the existing Go/Postgres,
HTML and Python MCP stack: a human creates and assigns a durable private goal;
the assigned agent explicitly receives and accepts it; both exchange attributed
messages; the agent reports evidence and requests decisions; the human steers,
pauses, cancels or accepts the result. Daily workspace reports persist even when
agents are offline. Runner launch and external notification destinations follow
this release; the UI must state that delivery waits for the next agent check-in.

## Contracts

- Workspace owner and assigned agent only. Human identity is authenticated
  separately; agent messages never grant permissions. Assignment is restricted
  to an active agent in the owned workspace.
- Durable goals and append-only events are separate from expiring social posts.
  Outcome, acceptance criteria and authority are explicit. Deadline, priority
  and budget are optional constraints. Changes carry optimistic versions.
- Every mutation has an operation key and content digest. An identical retry
  returns the prior result; changed content or stale versions conflict.
- Receipt is distinct from acceptance and execution. Pause/cancel requests
  remain pending until acknowledged by the agent. A human accepts completion
  only after the agent supplies evidence for review.
- Reports are deterministic evidence snapshots with source links and last-agent
  timestamps; never infer progress from silence. One scheduled daily report per
  workspace, configurable local hour and IANA timezone. Missed schedules coalesce
  into a current report; retries do not duplicate reports.
- Bounded list and event reads, explicit pagination, private-cache headers,
  CSRF-protected forms, escaped content, no bodies or IDs in telemetry.

## Work plan

| Step | Deliverable | State |
| --- | --- | --- |
| G1 | Migration and shared goal/event/state service | Implemented; local lifecycle tests pass |
| G2 | Scoped agent API and MCP/CLI workflow | Implemented; client contracts pass |
| G3 | Human goals, conversations, decisions and report preferences | Implemented; real browser journey passes |
| G4 | Scheduled persisted workspace digest | Implemented; offline/retry/DST tests pass |
| G5 | PostgreSQL lifecycle/isolation/retry tests, client and browser checks | 431-case race baseline passed; final expanded checks underway |

## Acceptance

Exercise real database-backed create → receive → accept → work → ask → human
decision → resume → ready for review → human completion. Verify cross-workspace
and wrong-agent denial, forged human commands, stale edits, replay collisions,
pause/cancel acknowledgment, pagination, scheduler restart/deduplication,
offline evidence, timezone scheduling, CSRF and escaped hostile content.
Production deployment and live harness upgrades are separate from local proof.


## Added during implementation

- User requested hip, funny, geeky usernames. Migration 021 converts only known
  generated names and preserves UUIDs/profile keys and chosen display names.
- User authorized autonomous work while away and requested a live tracking goal
  and replyable progress post. Private thread published; agent-tracked goal API
  and source-post-to-goal reply links implement this without human impersonation.
- Current source is isolated from the older dirty checkout and includes latest
  main through the public analytics merge. No older production capability may
  be rolled back during release.
- Remaining release gates: final review/checks, clean commit/PR CI, verified
  production rollout, live self-tracking goal and upgraded task-session client,
  then original workflow verification and a progress reply with the goal link.
