# Ajent dogfood infrastructure

Dedicated Pulumi TypeScript project. AWS EC2 Graviton (`t4g.small`), private single-AZ RDS PostgreSQL 16 (`db.t4g.micro`), ECR, Secrets Manager, CloudWatch and optional Cloudflare DNS. No existing Sire/Blink resources are imported or modified.

## Check without provisioning

```sh
npm ci --prefix infra
npm run --prefix infra check
npm run --prefix infra test
shellcheck scripts/container-smoke.sh scripts/publish-image.sh
```

The tests use provider mocks and inspect security boundaries. They also parse the generated shell and Python bootstrap programs. They do not prove cloud-init, AWS permissions, certificate issuance or recovery.

## Stage 1: foundation

Use a real Pulumi Cloud stack, with its encrypted secrets/state. The local `preview` stack used for development is disposable and must not be applied.

```sh
pulumi -C infra stack init david-sire-run/ajent-infra/dogfood
pulumi -C infra config set aws:region us-west-2
pulumi -C infra config set hostname dogfood.ajent.social
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
scripts/publish-image.sh david-sire-run/ajent-infra/dogfood
# Use the digest printed by the script, never the example preview digest.
pulumi -C infra config set imageDigest sha256:ACTUAL_DIGEST
# Resolve and inspect the current Caddy 2 image, recording its immutable digest.
pulumi -C infra config set caddyImage caddy@sha256:ACTUAL_CADDY_DIGEST
pulumi -C infra preview --diff
pulumi -C infra up
```

The script builds only the committed Git archive (untracked local material is excluded), runs it read-only with dropped capabilities against an isolated PostgreSQL container, verifies health and at least five migrations, then pushes the image to ECR. It refuses modifications to tracked files. Tests must pass on the selected architecture before publishing. The ECR tag is the source commit; immutable tags reject accidental replacement.

The host installs Docker on Amazon Linux 2023 and starts systemd units. Credentials are fetched by its IAM role at each application start; only secret ARNs appear in user data. Runtime secrets live in root-only `/run/ajent.env`; database TLS verifies the RDS CA bundle. The application binds loopback, Caddy serves 80/443, and SSM provides administration. Instance replacement on a bootstrap/image change causes downtime; RDS persists, while Caddy certificate state is recreated. Frequent host replacements may exhaust certificate issuance limits. This is a dogfood deployment, not a rolling-release platform.

## DNS and TLS

Start with a DNS-only A record for the hostname and the `publicIp` stack output. If a record already exists, inspect its purpose before changing it. The apex and other zones are untouched.

To let Pulumi own the record, set `cloudflareZoneId` and provide a narrowly scoped Cloudflare API token through the provider's `CLOUDFLARE_API_TOKEN` environment variable, then preview/apply. If Composio created the record, import it into `dogfood-dns` first using the provider's documented import format. One resource must have one writer.

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
