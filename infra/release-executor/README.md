# On-demand production release executor

This Pulumi YAML project owns the isolated executor and its private control
repository `ajent-social/production-release`. Existing application resources
remain owned by `david-sire-run/ajent-infra/production`; the existing hosted
release identity is unchanged. The executor stack is
`david-sire-run/ajent-release-executor/production`, account 394824061124,
us-west-2. YAML avoids a local provider-SDK compile during bootstrap.

## Credentials and isolation

A Fargate task runs one GitHub JIT runner registered ONLY to the private,
nonforkable control repository, then exits. The public application repository
cannot assign PR jobs to it. The former public release workflow is removed;
the private workflow is the single release scheduler. The launcher verifies
both the reviewed control SHA and the independently CI-approved application
SHA. The checked-in workflow template is `runners/release/workflow.yml`.

The task has no IAM task role, no inbound access and HTTPS-only egress. Its
execution role can pull only the runner image, write logs and read only the
one-use JIT parameter. Workflow OIDC trust permits only the control repo main
ref. The new deployment principal has exactly the approved existing production
release permissions captured in `deploy-policy.json`; neither identity has
secret-value reads or IAM management. The image-build identity can upload
versioned source bundles and start/stop only the application build project.
Build workers can publish only their respective application/runner ECR repos;
they cannot deploy production or read application secrets. Generic PR runners
are unchanged.

JIT configuration enters encrypted Pulumi config through stdin and SSM as a
SecureString. Never put it in ECS overrides, logs or receipts. The runner API
requires its encoded one-use configuration at startup. The launcher disables
the parameter after the attempt. Old SSM versions may hold expired/consumed
JIT configurations. Source archives include only committed files plus a source
marker; S3 versions are pinned, private, encrypted and expire after seven days.

The control repo needs a Pulumi backend token. On the current personal-account
setup this inherits the owner's Pulumi permissions; it is not project-scoped.
The owner must authorize this boundary explicitly. `scripts/release-token.py`
creates a separate three-hour token, installs it only as an encrypted config /
GitHub secret, and revokes its exact ID after the attempt before disabling the
secret. Existing owner tokens and the public repo's secret are unchanged.
No permanent personal token is copied into the new runner. A failed/unknown
cleanup is a reconciliation requirement, never a successful release claim.

## Bootstrap and release

1. Review exact source, policies and cost estimate. The program pins the AWS
   account; select an existing public subnet and matching VPC. No NAT gateway
   or persistent ECS service is created. Export GITHUB_TOKEN from existing
   local `gh` authentication only into the Pulumi process environment. Do not
   print it. Set encrypted jitConfig/pulumiAccessToken to `disabled`. Preview and
   inspect all changes, save and apply that plan. The protected private repo,
   reviewed workflow template and repo variables are managed by this stack.
2. Bootstrap the runner image using runnerBuildProject and a version-pinned
   S3 ZIP of the reviewed committed source. Its buildspec is
   `runners/release/buildspec.yml`; overrides RUNNER_REPOSITORY/RUNNER_REVISION
   select the dedicated immutable runner repo and source SHA. Record build ID,
   source SHA and digest. Set runnerImage to repository@digest, preview and
   apply. The initial placeholder image must NEVER be launched; the launcher
   rejects it and verifies the real task definition has no task role.
3. Merge independently reviewed application changes with required CI. Verify
   the control repo main workflow bytes match the reviewed template. Require
   successful exact-application-main push CI. The launcher will recheck this.
4. Acquire the existing R-ajent-apply lease, reconciling prior effects first.
   Install the authorized three-hour backend token with release-token.py.
   Run `python3 scripts/fargate-release.py --stack
   david-sire-run/ajent-release-executor/production --revision APP_MAIN_SHA
   --control-revision REVIEWED_CONTROL_MAIN_SHA --receipt /protected/attempt.json`.
   One workflow dispatch, one JIT registration and one ECS launch are journaled
   before effects. Never duplicate an unknown attempt.
5. Revoke the exact backend token and disable its secret after the attempt.
   Verify exact application revision / immutable image, Successful AWS rolling
   refresh and live health/OAuth/MCP probes. Release only the owned lease.
   Authenticated Muse/GrokBot acceptance is separate from these release checks.

The Fargate lifetime is capped at three hours. CodeBuild runner/application
projects have 30/60-minute limits, concurrency one and ten-minute queue limits.
No autoscaling webhook/daemon is installed; launches are owner-initiated.

## Costs

Idle compute is zero. A 4-vCPU/16-GiB Fargate runner is roughly $0.24/hour before
public IPv4, storage and logs; CodeBuild is billed while building. Initial
bootstrap plus release is estimated at $1-$3 depending on duration, not a
spending guarantee. Source/ECR/log storage and existing application costs
continue. No NAT gateway or permanently running runner is added. Record actual
elapsed usage and consult current regional pricing before future sizing.
