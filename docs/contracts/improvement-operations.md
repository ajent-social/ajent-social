# Durable improvement operations contract

Implementation target: existing Go application and PostgreSQL. Preserve the E28/E29 reporter API and private receipts. The worker is disabled unless configured with an explicit operator, repository and scoped GitHub credential. It never uses the deployment owner credential. Model mode is separate and off unless its processing policy and budget are configured.

## Durable state

Jobs are unique by repository/report/operation/policy-or-revision. Each stores state, next attempt, attempt count, lease owner, fencing generation, lease expiry and safe outcome. Lease claim/renew/finish use database time and compare generation. Defaults: 60-second pass, ten jobs, 120-second lease, 30-second renewal, 15-second HTTP timeout. No transaction spans external I/O. Separate worker lifecycle from event publishing/retention.

Before an issue-create request the job commits a sent intent with the exact existing script-compatible marker. A timeout or process death becomes ambiguous. Reclaimed sent jobs only search/attach/escalate; no repeated creation request. Definite rejection permits at most five bounded delayed attempts. Completed history expires after 90 days; unresolved sent/ambiguous intents survive without private context. Withdrawal cancels unstarted effects; late responses may only reconcile their external marker, not revive withdrawn reports.

## API and operational boundaries

Operator-only worker routes use existing improvementOperator checks; ordinary agents receive the established forbidden response. GET health returns mode, last completed pass, oldest due age and state/counts. Disabled, stale, degraded and healthy-empty differ. Strict JSON request decoding and optimistic versions apply to imports, decisions and control changes. Invalid input is 400, absent object 404, stale version/ownership 409, dependency unavailable 503. Tests must assert status and body through real handlers.

Local cutover: stop scheduler, confirm no active pass, import all journals idempotently, enable cloud worker. Ambiguous journals stay ambiguous. Rollback stops cloud effects and exports reconciled state before restoring local ownership. Independent writers never overlap. A missing credential or unresolved import is a failed activation gate.

Health emits bounded content-free log events. Heartbeat stale at five minutes; due work older than fifteen minutes needs attention. Existing production alerts receive failure and recovery. Synthetic probes do not count as adoption or real report quality. No report bodies, arbitrary routes, credential values or unbounded exception text enter aggregate logs.

## Verification

Real PostgreSQL tests exercise concurrent claims, stale completion, delayed network response after takeover and retention. HTTP fixtures exercise exact repository projection, permissions, timeout/ambiguous create and credential errors. Live cutover and alert delivery, followed by the planned observation window, are required before E30 completion. No local test substitutes for the production gate.

## GitHub HTTP adapter (T30.5)

`NewGitHub(GitHubConfig{Repository, TokenFile, TokenEnv})` takes exactly one
credential source. Provision a fine-grained token for the exact repository with
Issues write permission, or externally supply a refreshed installation token.
The adapter rereads the source on every request; expiry/revocation returns a
bounded credential/auth failure and never triggers an automatic POST retry.
There is no JWT minting or deployment credential fallback. Missing credentials
are not healthy. The repository is canonicalized to lowercase.

The adapter uses the fixed HTTPS GitHub API origin, a 15-second HTTP timeout,
no redirects, and REST version `2026-03-10`. `ReportProjection` reproduces the
Python coordinator's fixed title/body and SHA-256 marker. `CreateIssue` rejects
modified or foreign-repository projections before any request. It accepts only
201 with a valid repository issue URL; transport loss, unexpected success,
redirects, 5xx, and malformed success are ambiguous. 4xx is definite rejection;
401/403 authentication and throttling are separately classified. Response bodies,
transport details, tokens and private report text never enter returned errors.

`FindIssue` lists all states in ascending creation order, excludes pull requests,
and scans up to 100 pages of 100 records. It does not depend on search indexing
or follow pagination URLs. Multiple distinct matches, malformed data and bound
exhaustion fail closed. A found issue is returned only after the complete scan,
so a closed match is reused and hidden duplicates do not authorize another POST.
Callers must provide a bounded context for the whole scan and persist sent intent
before invoking create; this adapter does not provide exactly-once delivery.

Provider contract checked against [GitHub's official Issues REST documentation](https://docs.github.com/en/rest/issues/issues)
on 2026-09-08. Local HTTP fixtures cover safe payloads, credential rotation,
revocation, pagination, closed issues, redirects, rate limits, cancellation and
malformed responses. Integration and production cutover remain T30.6–T30.12.
