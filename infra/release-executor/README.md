# On-demand production release executor

This separate Pulumi YAML project owns only the release executor, not the
application fleet or the existing production deployment identity. YAML avoids
compiling the infrastructure provider SDK on the shared Mac during bootstrap.
Use the `production` stack in project `ajent-release-executor`, in AWS account
394824061124 / us-west-2. Existing application resources remain owned by
`david-sire-run/ajent-infra/production`.

## Credentials and isolation

A Fargate task runs one GitHub JIT runner with a unique release-run label, then
exits. It has no IAM task role, no inbound network access and HTTPS-only egress.
Its ECS execution role can pull only the runner image, write executor logs and
read only the one-use JIT parameter. Only the Actions job can obtain production
credentials through the existing exact-main GitHub OIDC trust. The separate
image-build role can upload versioned source bundles and start/stop only the
application image-build project. Its CodeBuild service role can publish only
the application's ECR repository. Neither build role can deploy production or
read application secrets. Generic PR runners are unchanged.

JIT configuration is entered through stdin as encrypted Pulumi config; SSM
stores it as SecureString. Do not put it in argv, ECS environment overrides,
receipts or logs. The launcher disables it after the bounded attempt. Old SSM
parameter versions may contain consumed/expired configurations; GitHub JIT
runners serve only one job. Source bundles contain only exact committed Git
archives and a source-revision marker, expire after seven days and use pinned
S3 object versions. Runtime application credentials never enter these bundles.

## Bootstrap and release

1. Independently review the exact source and IAM policies. Set vpcId/subnetId to
   an existing public subnet in the same VPC. No NAT gateway or persistent ECS
   service is created. Verify the AWS caller is the pinned account 394824061124 and
   applicationRepository is the existing production repository. Preview and inspect all changes,
   save the plan and apply it. The initial runnerImage is deliberately not
   runnable; never launch the task until replaced with the built digest.
2. Use runnerBuildProject to build `runners/release/Dockerfile` from a versioned
   committed-source ZIP in sourceBucket. Its source buildspec is
   `runners/release/buildspec.yml`. Set RUNNER_REPOSITORY and RUNNER_REVISION
   environment overrides. Pin the source S3 version. Record the CodeBuild ID,
   source SHA and immutable image digest. Set runnerImage to repository@digest
   through Pulumi, preview, inspect and apply.
3. Set GitHub repository variables AJENT_IMAGE_BUILD_ROLE_ARN,
   AJENT_IMAGE_BUILD_PROJECT, AJENT_RELEASE_SOURCE_BUCKET from stack outputs.
   Existing AWS_DEPLOY_ROLE_ARN, PULUMI_STACK and PULUMI_ACCESS_TOKEN remain in
   their existing custody. Merge reviewed source and require successful main
   push CI. Do not add production credentials to PR workers.
4. Hold the existing R-ajent-apply lease throughout the release. Reconcile any
   old rollout/update first. Run `python3 scripts/fargate-release.py --stack
   david-sire-run/ajent-release-executor/production --revision FULL_MAIN_SHA
   --receipt /protected/unique-attempt.json`. This dispatches once, launches
   one task and supervises its bounded execution. On unknown outcomes inspect
   the receipt/run/task before taking any action. Never duplicate a release.
5. Require exact-head workflow success, Successful ASG instance refresh,
   deployed immutable image provenance and live health/OAuth/MCP checks.
   Source, image smoke tests, deployment and authenticated client acceptance
   are separate evidence. Release only the exact owned lease token.

The 3-hour task timeout bounds idle/execution lifetime. CodeBuild projects have
concurrency one and 30/60-minute execution limits plus a 10-minute queue limit.
No webhook/autoscaling daemon is installed: launches are owner-initiated.

## Costs

Idle compute is zero. A 4-vCPU/16-GiB Fargate runner costs roughly $0.24/hour
before public IPv4, storage and logs; the ARM64 image builder is billed only
while building. Runner-image bootstrap is a separate on-demand x64 build.
Expect roughly $1–$3 for an initial build/release, subject to duration and
regional pricing, not a guaranteed cap. One 3-hour runner plus one maximum
application build and one bootstrap build is bounded by configured timeouts;
source/ECR/log storage and AWS application costs continue separately. No NAT
gateway or permanently running runner is added. Consult current AWS pricing
and record actual elapsed usage after execution.
