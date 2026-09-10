# Improvement worker infrastructure

T30.9 implementation, 2026-09-08. This change extends the currently deployed
TypeScript infrastructure. The separate infrastructure-language migration in
PR #8 remains independent; merge no unrelated migration during this rollout.

Configure `improvementRepository` as one lowercase `owner/repository`. Configure
`improvementGitHubSecretArn` as the identifier of an existing, dedicated Secrets
Manager secret whose raw string is a repository-scoped GitHub token. Provision the
credential separately through the approved secret workflow; never copy a local
broad `gh` credential. IAM adds GetSecretValue for that exact configured secret,
with no wildcard resource or new secret-management permissions.

Repository configuration alone is allowed for preparation. It is **not** an
activation pass: the scoped credential and confirmed alert destination are
required before cutover. Startup writes `AJENT_IMPROVEMENT_REPOSITORY` and, only
when successfully fetched and validated, `AJENT_IMPROVEMENT_GITHUB_TOKEN` into the
existing protected environment file. The token is never in Pulumi state, user
data, or logs. A 15-second timeout, empty/invalid value or failed fetch omits the
token and emits only a constant diagnostic; the application stays available and
the worker reports degraded. Secret updates take effect on application restart;
coordinate rotation with a rolling refresh, preserving application availability.

With a repository configured, two log filters feed existing SNS alerts and
recovery notifications:

- `improvement_worker` events supply numeric `healthy` (0 or 1). The minimum across
  replicas is evaluated over five one-minute periods. Missing data breaches. The
  filter has no default value: unrelated application logs cannot fabricate a
  successful heartbeat.
- Nonnegative `oldest_due_seconds` values use their maximum; 900 seconds triggers
  an overdue-work alert. Missing/unknown due-age values do not fabricate a queue;
  the heartbeat alarm separately covers unavailable evidence.

These filters only see events ingested after creation. Their behavior follows
[AWS metric-filter semantics](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/MonitoringLogData.html).
CloudWatch ingestion/evaluation introduces latency; the private health endpoint
remains the immediate source for the five-minute stale-state check.

Validation: TypeScript check passed; five Node tests and four existing Python
infrastructure tests passed, zero skipped or failed. Mock resource assertions cover
exact secret IAM, configured/unconfigured alarms, missing-data/recovery behavior,
rolling-launch configuration, and the EC2 user-data size limit. Executed embedded
Python covers seven credential success/failure/format cases without real secret
access. No production preview, apply, SNS delivery, or observation gate was
performed by this task; those remain T30.12 release evidence.
