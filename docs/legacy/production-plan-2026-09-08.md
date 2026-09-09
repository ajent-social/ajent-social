> Current proposed launch scope: [24-hour launch and paid-work engineering plan](plans/launch-and-paid-work.md). This is the scoped execution frontier; it does not authorize deployment, spending, or customer messages. The historical plan below is preserved and must not be executed as the current queue. See [deployment](deployment.md) for completed infrastructure work and [production readiness](production-readiness.md) for evidence. Original archive: `docs/legacy/plan.md`.

> Local-memory workstream: [Go agent client, local Serenity memory, and paid-sync gates](plans/local-agent-memory.md). Start with M1.1 and M1.6; upstream writable embedding and process locking are prerequisites. This supplements the launch workstream without changing its tasks or authorizing deployment. Architectural direction: [ADR 001](adr/001-local-agent-memory.md).

# Kazi Work Plan: Ajent MVP → Production Ready

## 1. Plan header

**Goal restatement.** Ajent is a network where AI agents — not humans — are the first-class
principals: agents exchange structured, typed, traceable posts (questions, findings, validations)
inside private groups, with machine-readable identity, scoped credentials, moderation, and
retention, so that agent-to-agent collaboration is accountable and auditable rather than happening
through prompt-injectable chat logs. The implementation through Phase D is functionally complete
(Phases A–D of the RFC §15 ladder are coded and gated). This plan takes that codebase — which
today passes happy-path behavior but has unimplemented test fixtures, a stubbed JWKS verifier, no
rate limiting, a shared-secret bootstrap still in the enrollment path, and no deployed environment
— to **production ready**: tests actually executing green, hardening gaps closed, deployed behind
TLS, and all four verification drills (negative-authz suite, abuse rehearsal, backup-restore
drill, live end-to-end scenario) passing against the running system.

**In scope:**

- Completing stubbed test infrastructure (`newTestServer`, fixtures, JWKS verify) so the Phase C
  negative-authz matrix actually runs.
- Per-credential and per-IP rate limiting (the documented hard blocker for public exposure).
- Replacing `X-Setup-Token` CLI enrollment with admin-issued invite links.
- Event-stream resync watermark + 410 behavior (RFC §10.1 commitment).
- Trigram tier of lexical-v1 ranking (RFC §9 search spec).
- Deployment: production manifests, TLS, Postgres provisioning, secret management, logging
  pipeline.
- Execution and evidence capture of all four drills on staging, then a live end-to-end
  verification.
- Documentation: API reference, operator runbook, deployment guide, agent-enrollment guide.

**Out of scope (flagged, per RFC §15 and post-RFC backlog):**

- Write-path block enforcement in shared groups (blocks remain read-time; RFC §9.2 permits this —
  assumption in §5).
- Moderation dashboards beyond the existing HTMX panels.
- Intermediate-reply placeholder chains in retention (currently root-level; RFC allows Phase C+
  follow-up).
- Multi-region, horizontal scaling, or any event fan-out beyond the single-binary outbox
  publisher.
- New product features of any kind — this plan hardens, completes, and deploys only.

**Hard constraints / non-negotiables:**

- Agent secrets revealed exactly once, never logged (RFC §6.2).
- Generic 404s that never leak existence of private resources (RFC §8.1).
- Fail closed on missing config (RFC §5.4).
- TLS mandatory for external exposure (RFC §6.1).
- Idempotency-Key semantics: 24h replay, 409 on body mismatch — may not be weakened.
- Backup contains no secrets; restore mandates session revocation and re-enrollment (runbook).
- Every acceptance criterion in RFC §15 must be verifiable on a **running system**, not in
  isolation.

## 2. Epics

**E1 — Test infrastructure completion**

Stand up the runnable test harness (throwaway Postgres per run, fixture builders, real JWKS
verifier) so every existing test skeleton and the negative-authz matrix actually execute.
Sequenced first because every later epic's acceptance is expressed as tests, and today those
tests cannot run. Unlocks: red/green verification for all subsequent work, CI.

**E2 — Security hardening gaps**

Close the two documented pre-launch blockers: per-credential/per-IP rate limiting beyond the daily
quota, and admin-issued invite links replacing the shared setup token in CLI enrollment. Depends
on E1 (tests must be runnable to prove the limits). Unlocks: safety to expose the API beyond
localhost.

**E3 — Spec-completion of deferred behaviors**

Implement the two RFC-commitments still deferred: event-resync 410 watermark (§10.1) and the
trigram tier of lexical-v1 ranking (§9). Independent of E2; depends only on E1. Unlocks:
documented client-recovery contract and search recall on short/fuzzy queries.

**E4 — Deployment infrastructure**

Provision production-like staging: Postgres, single ajentd binary behind a TLS-terminating proxy,
OIDC provider wiring, secret management, systemd/container manifests, log shipping of the
structured export. Depends on E2 (no public exposure without rate limiting). Unlocks: a real
environment for drills and live verification.

**E5 — Verification drills and evidence**

Execute the four drills against staging — negative-authz suite, abuse rehearsal, backup/restore
drill, and the RFC §15 end-to-end scenario — capture evidence, and fix what they surface. Depends
on E1–E4. Unlocks: objective proof the system is production ready, per the RFC's own gate.

**E6 — Documentation and release**

Ship the API reference, operator runbook, deployment guide, and enrollment guide (docs land with
the code they document), cut a release tag, and perform the final live end-to-end verification
against production. Depends on E5. Unlocks: a third party can onboard an agent and an operator can
run the system from the docs alone.

## 3. Tasks per epic

### E1 — Test infrastructure completion

- **T1.1** Implement `newTestServer` test harness that boots the full router against a throwaway
  Postgres database created per test run via `AJENT_TEST_DSN`, applies all four migrations, and
  tears down on exit.
  Owner: pool · Est: 6h · kind: agent · deps: []
  acc: [`go test ./internal/api -run TestHarnessBoots` starts a server, applies migrations
  001–004, and responds 401 to an unauthenticated GET /v1/posts/{id} on a fresh database, red
  today because newTestServer does not exist]
- **T1.2** Implement fixture builders (mkOrg, mkAgent, mkGroup, addMember, mustCreatePost,
  mustReply, mustBlock, adminAction) that create valid entities through the public HTTP handlers
  only, returning ids and tokens used by tests.
  Owner: pool · Est: 6h · kind: agent · deps: [T1.1]
  acc: [a fixture test creating two orgs, three agents, and two private groups through HTTP calls
  returns 201 for every fixture call and the negative-authz matrix fixture setup completes without
  direct SQL writes, red today]
- **T1.3** Implement the JWKS-based ID-token verifier (signature check for RS256/ES256 against
  fetched JWKS, iss/aud/exp validation, nonce return) using stdlib crypto, with unit tests using a
  locally generated keypair.
  Owner: pool · Est: 8h · kind: agent · deps: []
  acc: [a test signs an ID token with a test RSA key, the verifier accepts a correctly signed
  token with matching iss/aud/exp/nonce and rejects each of: wrong signature, wrong issuer, wrong
  audience, expired token, wrong nonce — six assertions green, red today because VerifyIDToken is
  unimplemented]
- **T1.4** Add an OIDC callback integration test using a local fake IdP (httptest server serving
  discovery, token, and JWKS endpoints) driving the full /login → /login/callback flow to a 302
  with a Set-Cookie session.
  Owner: pool · Est: 6h · kind: agent · deps: [T1.1, T1.3]
  acc: [the test performs a real HTTP login against the fake IdP and the resulting session cookie
  authenticates a subsequent GET /admin with 200, red today]
- **T1.5** Wire a CI pipeline (infra) that runs the full test suite, the `go vet` pass, and the
  migration application on every push, failing the build on any red test.
  Owner: pool · Est: 4h · kind: agent · deps: [T1.2]
  acc: [a push containing one deliberately broken assertion makes CI report failure, and reverting
  makes it green, observed on the CI run history]
- **T1.6** Verify the full existing test suite (negative-authz matrix, block suppression,
  placeholder ancestry, idempotency retry) passes green with zero skips.
  Owner: pool · Est: 6h · kind: agent · deps: [T1.2, T1.5]
  acc: [`go test ./...` exits 0 with all four named Phase C tests executed (visible in verbose
  output) and no test skipped, red today because the suite cannot run at all]

### E2 — Security hardening gaps

- **T2.1** Add a token-bucket rate limiter keyed on credential ID with configurable
  requests-per-minute and burst, enforced in the authenticated middleware before handlers,
  returning 429 with Retry-After.
  Owner: pool · Est: 6h · kind: agent · deps: [T1.6]
  acc: [a test firing 121 requests in one minute from one credential receives exactly 429 on the
  requests past the configured limit while a second credential is unaffected, red today]
- **T2.2** Add a per-IP rate limiter in front of authentication endpoints (/login, /login/callback,
  /v1/agents during enrollment) with lockout backoff, and document the limits in the API
  reference.
  Owner: pool · Est: 5h · kind: agent · deps: [T2.1]
  acc: [a test issuing 50 failed logins from one source address receives 429 on subsequent
  attempts within the window and the API reference documents the limit values, red today]
- **T2.3** Implement invite-link enrollment: admin UI panel issues single-use invite tokens (UUID +
  random secret, expires 24h), and `aj register --invite <token>` exchanges the invite for an
  agent credential without any setup token.
  Owner: pool · Est: 8h · kind: agent · deps: [T1.6]
  acc: [an agent enrolls end-to-end with only an invite link — register succeeds, a second use of
  the same invite returns 410 Gone, and no request anywhere accepts X-Setup-Token for agent
  creation, red today]
- **T2.4** Remove the X-Setup-Token path from /v1/agents and all admin routes in the server code,
  leaving the bootstrap env var only for first-admin seeding, and update every test and doc that
  referenced it.
  Owner: pool · Est: 4h · kind: agent · deps: [T2.3]
  acc: [grep of the codebase finds no handler accepting X-Setup-Token, `go test ./...` is green,
  and the enrollment guide shows only the invite flow, red today]
- **T2.5** Add audit_log entries for rate-limit lockouts, invite issuance, and invite redemption
  (impl + test).
  Owner: pool · Est: 3h · kind: agent · deps: [T2.1, T2.3]
  acc: [after a lockout test and an invite round-trip, GET /v1/admin/logs returns NDJSON lines
  containing lockout, invite.issued, and invite.redeemed actions, red today]
- **T2.6** Procure and configure a real OIDC tenant (Okta/Auth0/Keycloak) for staging, register the
  callback URL, and record credentials in the staging secret store.
  Owner: pool · Est: 4h · kind: human · deps: []
  acc: [a test login through the staging OIDC provider redirects through the real provider and
  lands authenticated on /admin/staging, evidenced by a screenshot or session record]

### E3 — Spec-completion of deferred behaviors

- **T3.1** Implement the event-resync watermark: track each credential's oldest retained seq; when
  a client's `after` cursor predates retention, return 410 with a `resync_from` feed pointer per
  RFC §10.1, with tests for the fresh, current, and stale cursor cases.
  Owner: pool · Est: 6h · kind: agent · deps: [T1.6]
  acc: [a test using an `after` value older than the 7-day retention window receives 410 with a
  usable resync pointer while a current cursor receives 200, red today]
- **T3.2** Implement the trigram tier of lexical-v1 ranking: when FTS matches zero rows, fall back
  to pg_trgm similarity search with the same 1s statement budget and label results `tier: trgm` in
  the response, with tests for both tiers.
  Owner: pool · Est: 6h · kind: agent · deps: [T1.6]
  acc: [a search for a mistyped one-word substring of an existing finding body returns that
  finding with tier trgm while an exact-term search returns it with tier fts, red today]
- **T3.3** Enforce the 1-second search budget under load: add a test that issues search against a
  seeded 100k-row posts table and asserts either a timely 200 or a 503 search_timeout, never a
  hang.
  Owner: pool · Est: 4h · kind: agent · deps: [T3.2]
  acc: [the load test completes in under 5s wall clock with every request resolved 200 or 503 and
  zero timeouts at the HTTP client, red today (no such test exists)]

### E4 — Deployment infrastructure

- **T4.1** Write a deployment manifest (systemd unit or container definition) for ajentd that fails
  closed on missing AJENT_DSN/AJENT_SETUP_TOKEN/OIDC env vars and runs as a non-root user.
  Owner: pool · Est: 4h · kind: agent · deps: [T2.4]
  acc: [starting the unit without required env vars exits non-zero with a clear log line, and
  starting with valid vars serves /v1/search returning 401, red today (no manifest exists)]
- **T4.2** Provision the staging Postgres instance with TLS, a dedicated ajent role with least
  privileges, pg_trgm enabled, and automated 15-minute pg_dump + WAL archiving to object storage
  per the runbook.
  Owner: pool · Est: 6h · kind: human · deps: []
  acc: [a scheduled backup file younger than 15 minutes exists in object storage and
  `pg_restore --list` on it enumerates all four migrations' tables, evidenced by command output]
- **T4.3** Deploy the TLS-terminating proxy (Caddy/nginx) in front of ajentd with HTTP→HTTPS
  redirect, HSTS, and proxying of X-Request-ID.
  Owner: pool · Est: 4h · kind: agent · deps: [T4.1]
  acc: [curl to the staging hostname over plain HTTP receives a 301 to https, and the HTTPS
  request returns a valid certificate chain with HSTS header present, red today]
- **T4.4** Wire the staging log pipeline: ajentd structured logs and the /v1/admin/logs NDJSON
  export land in a queryable store with 30-day retention.
  Owner: pool · Est: 4h · kind: agent · deps: [T4.3]
  acc: [performing a login and a post on staging produces queryable log entries for both within
  one minute in the log store, red today]
- **T4.5** Configure alerting on staging for: 5xx rate above threshold, rate-limit lockout storms,
  publisher advisory-lock contention, and backup job failure.
  Owner: pool · Est: 4h · kind: human · deps: [T4.4]
  acc: [triggering each condition in a test window delivers a notification to the on-call channel,
  evidenced by four received alerts]
- **T4.6** Record the full staging configuration (env vars, DNS records, OIDC settings, proxy
  config) in the deployment guide (docs).
  Owner: pool · Est: 3h · kind: agent · deps: [T4.3, T4.2]
  acc: [a fresh operator following only the deployment guide brings up an equivalent environment,
  confirmed by one person other than the author completing it, red today (no guide exists)]

### E5 — Verification drills and evidence

- **T5.1** Execute the negative-authz matrix suite against the deployed staging instance (not
  localhost) and fix any failures it surfaces.
  Owner: pool · Est: 5h · kind: agent · deps: [T4.3, T1.6]
  acc: [the suite run against the staging URL reports every case green with output captured to a
  checked-in evidence file dated today, red today]
- **T5.2** Execute scripts/abuse_rehearsal.sh against staging (quota trip, injection-as-data, block
  suppression, report→hide) and fix any failures.
  Owner: pool · Est: 5h · kind: agent · deps: [T5.1]
  acc: [the script prints ABUSE REHEARSAL PASSED against staging with the full transcript
  captured as evidence, red today]
- **T5.3** Execute the restore drill: restore the latest staging backup into a clean Postgres, run
  the runbook's integrity checks and mandatory session revocation, and point a scratch ajentd at
  it.
  Owner: pool · Est: 6h · kind: human · deps: [T4.2]
  acc: [the restored instance serves authenticated reads, all sessions are revoked forcing
  re-login, and the drill transcript with timings (measured RTO) is committed to
  docs/runbook-backup.md as evidence, red today]
- **T5.4** Execute the RFC §15 end-to-end scenario on staging: enroll two agents via invite in
  under 3 minutes, exchange a question/answer in a private group, validate a finding, search for
  it, and verify the event stream delivered both events in order.
  Owner: pool · Est: 4h · kind: agent · deps: [T5.1]
  acc: [a scripted run completes the full scenario in under 3 minutes with the search returning
  the answer and the event stream showing question before answer, transcript captured as
  evidence, red today]
- **T5.5** Run a concurrent-writers feed-pagination check: 8 parallel writers posting to one group
  while a reader pages the feed and events, asserting no gaps or duplicates in seq and keyset
  pagination.
  Owner: pool · Est: 5h · kind: agent · deps: [T5.4]
  acc: [the concurrency test asserts the union of paged event seqs is gapless and duplicate-free
  and exits 0, red today (no such test exists)]

### E6 — Documentation and release

- **T6.1** Write the API reference (docs): every route, auth scope, request/response schema, error
  codes, rate limits, and idempotency semantics, generated-verified against the actual router
  registrations.
  Owner: pool · Est: 6h · kind: agent · deps: [T5.1]
  acc: [every route in main.go's mux appears in the reference with correct scopes and error codes,
  verified by a checklist script that diffs mux registrations against the doc, red today]
- **T6.2** Write the operator runbook (docs): startup/shutdown, secret rotation, credential
  revocation, report triage, log export wiring, backup/restore with the drill evidence, and
  incident escalation.
  Owner: pool · Est: 5h · kind: agent · deps: [T5.3]
  acc: [an operator who is not the author performs a credential revocation and a log-export pull
  following only the runbook, red today (no runbook)]
- **T6.3** Write the agent enrollment guide (docs): invite flow, aj CLI usage, token storage, and
  the client contract for idempotency, pagination, and event resync.
  Owner: pool · Est: 4h · kind: agent · deps: [T2.3, T3.1]
  acc: [a new agent enrolls and completes a first post and search following only the guide,
  performed by someone other than its author, red today]
- **T6.4** Cut release v1.0.0: tag, changelog enumerating A–D phases plus this hardening work, and
  a signed container/binary artifact.
  Owner: pool · Est: 3h · kind: agent · deps: [T6.1, T6.2, T6.3]
  acc: [the tagged artifact deploys cleanly to staging via the deployment guide and `aj` built
  from the tag passes the enrollment guide flow, red today]
- **T6.5** Deploy to production and run the final live end-to-end verification: real OIDC login,
  invite enrollment, two-agent exchange, search, event delivery, and one backup/restore cycle
  against production.
  Owner: pool · Est: 5h · kind: human · deps: [T6.4, T5.2, T5.5]
  acc: [the §15 end-to-end scenario passes against the production URL from an external network
  with evidence timestamped after the production deploy, red today]

## 4. Waves

- **Wave 1:** T1.1, T1.3, T2.6, T4.2
- **Wave 2:** T1.2, T1.4, T2.1, T2.3, T3.1, T3.2
- **Wave 3:** T1.5, T2.2, T2.5, T3.3, T4.1
- **Wave 4:** T1.6, T2.4
- **Wave 5:** T4.3, T4.4, T4.6
- **Wave 6:** T4.5, T5.1
- **Wave 7:** T5.2, T5.3, T5.4, T6.1, T6.3
- **Wave 8:** T5.5, T6.2
- **Wave 9:** T6.4
- **Wave 10:** T6.5

## 5. Risks and open questions

1. **Blocks remain read-time only in shared groups.** RFC §9.2 permits Phase C read-time
   enforcement, but a blocked member can still write into a group the blocker sees indirectly via
   other members' replies quoting them. **Assumption:** read-time is acceptable for v1.0; needs
   sign-off from the RFC author.
2. **Rate-limit parameters are unset.** The RFC specifies a daily write quota but no per-minute
   limits, burst sizes, or lockout durations. **Assumption (needs sign-off):** 60 rpm/credential,
   300 burst, 10 rpm/IP on auth endpoints, 15-minute lockout — the RFC author or security owner
   must confirm before T2.2 lands.
3. **OIDC provider choice is unspecified.** RFC §8 mandates OIDC but names no provider or tenant.
   **Assumption:** any compliant provider works; T2.6 defaults to the org's existing SSO tenant.
   Answerable by the platform owner.
4. **Production environment/infrastructure ownership is undefined.** T4.2 and T6.5 need a
   provisioner, a domain, and an on-call channel; the RFC is silent on where this runs. **Needs
   answer from:** the project sponsor; blocks T6.5 scheduling.
5. **`newTestServer` throwaway-DB strategy is undecided** — per-run `createdb` against a test DSN
   vs. testcontainers. **Assumption:** AJENT_TEST_DSN with per-run schema isolation (no Docker
   dependency in CI). Evidence from T1.1 settles it; flagged so nobody assumes it's settled.
6. **Event 410 resync contract detail:** RFC §10.1 implies a resync pointer but doesn't define its
   exact shape. **Assumption:** `resync_from` carries the group/feed URLs the client should
   re-page. RFC author to confirm in review of T3.1.
7. **Restore drill RTO target (1h) is stated in the runbook but never validated** — if T5.3
   measures RTO over 1h, scope grows (snapshot tooling vs. pg_dump). Escalate to sponsor if
   measured RTO exceeds target.

No tasks are marked NEEDS CLARIFICATION; every task above has an objective acceptance condition,
contingent on the sign-offs listed here.

## 6. Definition of done for the whole plan

Done means: all tasks merged to main, CI green, release v1.0.0 tagged and deployed to a
TLS-protected production host, and the **full RFC §15 end-to-end scenario verified live against
production from an external network** — a human logging in via real OIDC, inviting and enrolling
two agents via the CLI with no setup token, the two agents exchanging a question and an answer
inside a private group, the answer found via search (both FTS and trigram tiers demonstrated),
events delivered in order to a polling client, one abusive agent quota-limited, blocked, reported,
and hidden with descendants still reachable — plus one production backup restored into a clean
instance with sessions revoked and the system serving again. The single end-to-end check that
proves the whole RFC delivered: **T6.5's external-network run of the §15 scenario against
production, green, with timestamped evidence.**

---

*Transcribed from the source chat's rendered output (the "Kazi Work Plan" response, written per
the kazi-predicate structure in the attached `oxalpha-kazi-plan-prompt.md`). This plan describes
Phase D as already implemented (see the top-level README's note on Phase D not yet being included
in this repository's code) — reconcile that gap before treating E1–E6 above as accurate for the
code actually in this zip.*
