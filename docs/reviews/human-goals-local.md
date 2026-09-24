# Human goal workflow — local evidence

2026-09-12 UTC. Source is the human-agent-goals feature branch based on main's
public analytics merge. No production rollout is established by this record.

The first full native PostgreSQL race run executed 431 test/subtest records with
zero skips and failures. After pagination/concurrency expansion, a final run is
required before release. The Python onboarding suite executes 76 passing cases,
including goal write retries, unsupported capabilities, invalid human actions
and bounded large reads.

Fourteen real-browser checks passed through the production HTTP server and a
local PostgreSQL database, with synthetic WebAuthn and agent credentials:
account creation/linking; explicit assignment; receive/accept/start distinctions;
human decision round trip; draft refresh/recovery; acknowledged pause/resume;
evidence then human completion; timezone settings; layouts at 390/1024/1440/2880;
keyboard focus; and axe checks with zero serious/critical WCAG violations on
goal detail, create and reports pages. Axe is injected only by the test harness;
the production CSP remains restrictive. A screenshot review confirmed the
mobile conversation layout. This is synthetic mechanics, not customer adoption.

The first browser attempt caught an ambiguous implicit action label. An explicit
label fixed the control; the full journey then passed. Scheduler tests caught a
PostgreSQL timestamp-parameter inference error; an explicit timestamptz cast
fixed it. An early web suite also caught missing OpenAPI operations; the route
catalog now covers the new endpoints.

The graph has no indexed goal nodes in the isolated checkout. Its zero-risk
report is missing coverage, not a successful feature review. Review relies on
source inspection and executed database/client/browser tests.

The 24-hour operational baseline was retained privately outside the repository.
It contains mixed release/client coverage and request counts, not distinct users.
Do not interpret it as adoption evidence or as a post-release comparison.
