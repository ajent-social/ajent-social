# Ajent infrastructure

Current production path: ACM TLS → Application Load Balancer → two Graviton hosts across two availability zones, with bounded autoscaling to four. RDS remains single-AZ. `rollingService=true` and `retireLegacyHost=true` are enabled for production. The legacy stages below document initial provisioning; use the rolling release procedure for updates.

## Rolling releases

After the one-time Go infrastructure migration is complete, publish routine releases through the private `ajent-social/production-release` control repository. Its reviewed workflow accepts only an exact, CI-approved current `main` commit of this application. The public application's former release workflow is removed so it cannot become a second scheduler. See [release executor](release-executor/README.md) for the bounded owner launcher. The workflow builds and smoke-tests the ARM64 image, publishes it by immutable digest, applies the Pulumi update without refreshing unrelated production resources, waits for the AWS refresh and verifies production endpoints. A failed check fails the release run.

The workflow runs on a dedicated, one-job Fargate runner, accepts AWS credentials only through GitHub OIDC, and serializes production releases. The owner launches the bounded executor with `scripts/fargate-release.py`; see [release executor](release-executor/README.md). Container builds and smoke tests run on isolated CodeBuild ARM64 workers, never inside Fargate. It intentionally does not use the self-hosted test runners, which execute pull-request code. Do not run concurrent manual production applies after enabling this release path; limit the OIDC role to the private control repository's `main` ref and remove routine production credentials from other release paths. This repository's current GitHub plan does not permit branch protection or required-reviewer environment protection. Dispatch permission and explicit confirmation are the available release gate; they are not an independent second-person review.

The executor IaC configures the private control repository. Its workflow uses repository variable `AWS_DEPLOY_ROLE_ARN`, repository variable `PULUMI_STACK`, and repository secret `PULUMI_ACCESS_TOKEN`. The new executor deployment role trusts only the private control repository's `main` branch through `token.actions.githubusercontent.com` with audience `sts.amazonaws.com`. Grant only image publication and rolling updates for the tagged production launch template and Auto Scaling group; do not grant secret-value reads, IAM management, or broad infrastructure writes. Routine preview and apply use `--refresh=false`, so the release role cannot read Secrets Manager values while reconciling unrelated resources. Handle infrastructure drift through a separate reviewed operator workflow. Do not use long-lived AWS keys. Prefer project-scoped Pulumi credentials when the backend supports them. The current personal-account route requires explicit approval for a separate three-hour personal token, revocation after the attempt and disabling its GitHub secret; it is not project-scoped.

The first Go apply is deliberately not part of the release workflow. Follow [Enforced first Go apply](#enforced-first-go-apply) with the TypeScript and Go no-change previews captured from the same settled stack state and the `R-ajent-apply` lease held throughout. The standard release workflow refuses to proceed until `goMigrationRevision` is recorded by that procedure. This separation prevents an image release from being combined with the runtime migration.

For diagnosis or a controlled recovery, the workflow invokes `scripts/codebuild-image.py` for the tested image and the existing `scripts/deploy.sh` for the rollout. The older `scripts/publish-image.sh` remains an operator reference and is not run inside Fargate. Keep the source commit, immutable image digest, workflow run and AWS instance-refresh result together as release evidence.

The steps below describe the underlying workflow.

1. Run application CI, `(cd infra && go vet ./...)`, and `(cd infra && go test ./...)`. Publish the tested ARM64 image using `scripts/publish-image.sh`.
2. Set `imageDigest` to its immutable digest. Run `pulumi -C infra preview --stack production --diff`; an ordinary release updates the launch template and autoscaling group, never the database or HTTPS certificate.
3. Apply with `pulumi -C infra up --stack production`. **Pulumi returning success does not mean the rolling refresh finished.**
4. Run `python3 scripts/wait-rollout.py "$(pulumi -C infra stack output autoscalingGroup --stack production)"`. Require `Successful`; rollback is a failed release. Check the refresh start time belongs to this deployment.
5. Verify trusted public HTTPS, authenticated reads, login, writes, search and revoked-key rejection. Monitor the public endpoint throughout the rollout.

The fleet keeps 100% healthy capacity, permits 150% during replacement, waits 120 seconds for warmup, and drains targets for 30 seconds. Baseline is two hosts; CPU target tracking uses 60% with a four-host normal maximum (maintenance can temporarily exceed this). Each host caps its database pool at four connections. New targets must pass database-backed `/healthz`. Failed refreshes have AWS automatic rollback enabled. Always use backward-compatible expand/contract database migrations: an image rollback cannot undo a destructive schema change.

TLS lives in ACM, independent of hosts. Cloudflare owns a DNS-only CNAME from `ajent.social` to `loadBalancerDns` and a permanent ACM validation CNAME. Native Cloudflare MCP manages these records; import the existing service record before enabling `cloudflareZoneId`; the optional provider resource uses CNAME in rolling mode. The application trusts only the two load-balancer subnet CIDRs, and its port 8080 accepts ingress only from the edge security group.

This increases baseline costs: two instances/EBS volumes, ALB hours and usage, and public IPv4 charges. No NAT gateway is required. RDS remains a single-AZ availability bottleneck; database storage autoscaling is not compute or connection autoscaling. SNS email delivery still requires configuring and confirming `alertEmail`.

## Historical foundation and single-host bootstrap

Dedicated Pulumi Go project. AWS EC2 Graviton (`t4g.small`), private single-AZ RDS PostgreSQL 16 (`db.t4g.micro`), ECR, Secrets Manager, CloudWatch and optional Cloudflare DNS. No existing Sire/Blink resources are imported or modified.

## Check without provisioning

```sh
(cd infra && go mod download)
(cd infra && go test -vet=off ./...)
# Shared-runner CI uses: go test -tags infra_ci -vet=off ./...
# That tag excludes generated pulumi-aws/cloudflare packages (OOM on the runner)
# and still runs bootstrap/config tests. Full provider parity tests run locally without the tag.
shellcheck scripts/container-smoke.sh scripts/publish-image.sh
```

The tests use provider mocks and inspect security boundaries. They also parse the generated shell and Python bootstrap programs. They do not prove cloud-init, AWS permissions, certificate issuance or recovery.

## Stage 1: foundation

Use a real Pulumi Cloud stack, with its encrypted secrets/state. The local `preview` stack used for development is disposable and must not be applied.

```sh
pulumi -C infra stack init david-sire-run/ajent-infra/production
pulumi -C infra config set aws:region us-west-2
pulumi -C infra config set hostname ajent.social
# Set alertEmail to the operator's actual notification address.
pulumi -C infra config set alertEmail YOUR_EMAIL
pulumi -C infra preview --diff
# After reviewing resources and costs:
pulumi -C infra up
```

This first stage creates database/network/image storage/secrets/alarms but no application host or DNS record. Confirm the SNS email subscription. Provisioning has ongoing AWS costs even before the host exists. Price the preview in AWS Pricing Calculator: 730 hours of db.t4g.micro, 20 GiB RDS gp3 (autoscaling capped at 100 GiB), secrets, logs, five alarms once fully enabled, and backup retention; stage two adds 730 hours of t4g.small, 20 GiB EBS gp3, one public IPv4 address, ECR storage and transfer. Burstable CPU charges, logs and traffic depend on usage. No NAT gateway or load balancer is provisioned. The provider preview is not a cost quote.

## Stage 2: tested image and host

Commit the source, run CI, then publish from a clean checkout. Docker is the default; `CONTAINER_ENGINE=podman` is supported. Build/smoke testing requires an ARM64 runtime (or correctly configured emulation).

```sh
scripts/publish-image.sh david-sire-run/ajent-infra/production
# Use the digest printed by the script, never the example preview digest.
pulumi -C infra config set imageDigest sha256:ACTUAL_DIGEST
# Resolve and inspect the current Caddy 2 image, recording its immutable digest.
pulumi -C infra config set caddyImage caddy@sha256:ACTUAL_CADDY_DIGEST
pulumi -C infra preview --diff
pulumi -C infra up
```

The script builds only the committed Git archive (untracked local material is excluded), runs it read-only with dropped capabilities against an isolated PostgreSQL container, verifies health and at least five migrations, then pushes the image to ECR. It refuses modifications to tracked files. Tests must pass on the selected architecture before publishing. The ECR tag is the source commit; immutable tags reject accidental replacement.

The host installs Docker on Amazon Linux 2023 and starts systemd units. Credentials are fetched by its IAM role at each application start; only secret ARNs appear in user data. Runtime secrets live in root-only `/run/ajent.env`; database TLS verifies the RDS CA bundle. The application binds loopback, Caddy serves 80/443, and SSM provides administration. Instance replacement on a bootstrap/image change causes downtime; RDS persists, while Caddy certificate state is recreated. Frequent host replacements may exhaust certificate issuance limits. This is a production deployment, not a rolling-release platform.

## DNS and TLS

Start with a DNS-only A record for the hostname and the `publicIp` stack output. If a record already exists, inspect its purpose before changing it. The apex and other zones are untouched.

To let Pulumi own the record, set `cloudflareZoneId` and provide a narrowly scoped Cloudflare API token through the provider's `CLOUDFLARE_API_TOKEN` environment variable, then preview/apply. If Composio created the record, import it into `production-dns` first using the provider's documented import format. One resource must have one writer.

No Composio tool was available in the implementation session. The optional record deliberately sets `proxied: false`. Do not simply toggle the orange cloud: first implement and verify Cloudflare proxy trust, cache bypass for authenticated responses, and Full (strict) origin validation. Current Caddy accepts direct traffic and the app trusts only its loopback reverse proxy.

## Acceptance on the host

Use `aws ssm start-session --target INSTANCE_ID --region us-west-2`. Check `systemctl status ajent-app ajent-proxy ajent-metrics.timer` and `/var/log/cloud-init-output.log`. Do not print `/run/ajent.env` or secret values in shared terminals/logs.

1. External HTTPS `/healthz` returns 200 with a valid certificate. The one-minute HTTPS metric checks the public hostname, so missing DNS or broken TLS raises an alarm.
2. Confirm CloudWatch app/proxy streams, disk and health metrics, and actual SNS alarm delivery; an unconfirmed email subscription delivers nothing.
3. Retrieve the owner key privately from the `ownerSecretArn` output in Secrets Manager; sign in, issue separate agent keys, and retain the owner key only with the human operator.
4. Run the private-group two-agent exchange and denied-read/revocation/retry checks in ../docs/hosting-plan.md. No test data should be presented as real agent learning.
5. Restart the application and verify persistence. Perform a point-in-time RDS restore into a separate private instance, clear browser sessions, rotate the restored owner key, and test with an isolated application host. Record achieved recovery time and data-loss window before relying on the service.

## Rotation and removal

Passwords are Pulumi-generated secrets stored encrypted in Pulumi state and Secrets Manager. Automatic database-password rotation is not enabled. For planned rotation, coordinate the RDS password and secret resource replacement, then restart the app through SSM so it reloads the secret; test new connections. Rotate the owner-key random resource and restart the app to invalidate owner browser sessions. Never change Secrets Manager alone while leaving Pulumi/RDS values inconsistent.

Database and secret resources are Pulumi-protected. RDS also has deletion protection and requires a final snapshot. Removal requires an explicit reviewed teardown, not a casual `pulumi destroy`. The final snapshot name must be changed if a snapshot from an earlier teardown already exists. Keep production stack state; losing it loses resource ownership and password recovery metadata.

### Optional OpenRouter chat

Store the OpenRouter API key as the plaintext value of an AWS Secrets Manager
secret, then set `openrouterSecretArn` to its ARN. Do not place the key itself in
Pulumi configuration or user data. Runtime IAM grants access only to that secret
alongside the existing database and owner secrets. Bootstrap fetches the key into
`/run/ajent.env` with private permissions before starting the container. Without
this configuration, Ask Ajent uses its explicitly labeled published-guide
fallback. See `docs/ai-chat.md` for quotas, model settings, and data boundaries.

## Go program and language migration

Run Pulumi from this directory on the authenticated CI runner; `Pulumi.yaml` selects the Go runtime. The nested
`go.mod`/`go.sum` pin AWS, Cloudflare, Random and Pulumi SDK versions independently
from the application. Node/npm are no longer needed for infrastructure checks.

On the shared mini, **do not run Go infrastructure builds, tests or previews**:
the generated AWS SDKs exceed its memory budget. Use the infrastructure
CI job; it limits compiler concurrency and memory and preserves dependency caches.
The commands in this document are CI-runner commands. `scripts/deploy.sh` requires
`CI=true`; an authenticated deployment runner and the existing lease are required
before applying. Do not export CI locally to bypass this restriction.
Host bootstrap helpers remain Python programs embedded by Go; their output is
byte-for-byte compatible with the former TypeScript generator.

DNS uses the supported Pulumi resource-registration API with Cloudflare provider
6.20.0 explicitly pinned, preserving its logical name, type, inputs and historical
name/type aliases without compiling the generated SDK for unrelated Cloudflare products.
The same DNS registration runs in CI and production. The infrastructure CI job
now compiles the full production program and runs its resource-parity fixtures
before a release can build or publish an application image.

Resource constructors keep the original logical names and provider versions.
Database, secrets and certificate protection, ASG desired-capacity ownership,
rolling-update dependencies and stack output names are preserved. Regression
fixtures in `testdata/` compare all resource inputs in foundation, legacy,
transition and rolling modes against the original TypeScript program.

For the runtime switch, first finish the existing resource imports, canonical
stack configuration/lease work, and the separately owned apex-domain cutover. Rebase this migration onto that state, retain the
same stack and configuration, and record TypeScript and Go previews against the
same state. Go must show zero changes/replacements and the same resource URNs.
The first Go apply must be a no-op under the deployment lease. Do not combine the
runtime migration with an image update, provider upgrade or resource refactor.

Capture both previews in private files on the authenticated CI runner, using
`--refresh=false --json --show-sames --expect-no-changes` against the same settled
stack state. Then run:

```sh
python3 scripts/check-infra-migration.py typescript-preview.json go-preview.json
```

The checker requires complete unchanged-resource reports and compares URNs,
provider references and protection flags. Do not publish raw preview JSON: it
contains stack configuration and resource metadata. Keep only the summary in the
PR. A provisional pre-cutover preview does not satisfy the final migration gate.

### Enforced first Go apply

Hold `R-ajent-apply` from the TypeScript baseline capture through the Go apply.
On the authenticated CI runner, with unchanged stack configuration and image,
run `scripts/deploy.sh STACK --migrate /private/path/typescript-preview.json`.
The command runs infrastructure tests, captures a fresh no-change Go preview,
compares the receipts, and applies only its saved no-change plan. It records
`goMigrationRevision` in stack configuration only after success and exits without
changing the image. Do not set this marker manually. Subsequent image releases
require that verified revision in their ancestry. Preserve the private receipt
directory as release evidence; never commit its contents.
