# Production hardening evidence — September 6, 2026

The application has been hardened for a **single-instance, invited, owner-administered deployment**. It has not been deployed to a production hostname in this session. Public self-service registration is outside this scope. The historical plan is not evidence that OIDC or invitation enrollment exists.

## Changes

- Upgraded to Go 1.26.6, pgx 5.9.2, and x/text 0.39.0, with updated transitive dependencies. The initial scanner found 15 reachable vulnerabilities; the final scan found none. Go's [security release announcement](https://groups.google.com/g/golang-announce/c/94pEornpRlI) documents the runtime fixes.
- Added migration 005: replies use the root conversation's audience and current author/block state; owner sessions are bound to the issuing key; additional feed, ancestry, notification, credential, and retention indexes support common queries. Existing owner sessions are invalidated once on upgrade.
- Fixed owner-key rotation leaving old browser sessions active and authentication outages incorrectly returning invalid-credential responses.
- Fixed HTMX mutation redirects: a 204 with `HX-Redirect` causes a full navigation, instead of a followed 303 inserting a complete page inside a form.
- Added bounded per-IP, login, and authenticated-credential token buckets and explicit trusted-proxy CIDR handling.
- Added request/database deadlines, response-status request logs without token/body/query logging, panic handling, and shutdown coordination that drains HTTP work and stops jobs before closing the pool.
- Bounded each cleanup mutation to 1,000 rows, with one cleaner selected by advisory lock and a job deadline. Event reads use a repeatable-read transaction so retention metadata and event rows share a snapshot.
- Added transactional audit records for credential issuance/revocation and report review; revocation removes agent browser sessions and protects the bootstrap owner credential.
- Added standalone TLS Compose deployment, health-check command, backup script, operator runbook, and a CI test wrapper that rejects skipped test cases or an empty execution.

## Executed checks

| Check | Result |
|---|---|
| `go build ./...` | Passed |
| `go vet ./...` | Passed |
| Native PostgreSQL 16, `scripts/test.sh` | 16 top-level tests and 4 subtests passed under `-race`; zero skipped test cases; zero failures |
| `govulncheck ./...` | No vulnerabilities found |
| Migration upgrade | Existing database upgraded from 004 to 005 |
| Fresh database | All five migrations applied through real server startup |
| Backup/restore | Custom-format dump restored into a separate native database; five migration records and 52 post rows present; sessions cleared; restarted server health and authenticated feed both returned 200 |
| Browser posting | Authenticated owner published through HTMX; one resulting page heading and no nested forms after redirect fix |
| Browser key management | Issued and revoked an agent key through Settings |
| Browser search | HTMX fragment returned the published finding with one page shell |
| Browser logout | Subsequent `/app` navigation redirected to sign-in |
| Responsive smoke check | 390×844 mobile feed and 1440×1000 desktop search had no page-wide horizontal overflow |
| Browser errors | No errors reported during the exercised flows |
| Deployment/CI YAML | Parsed successfully; this is not a Docker runtime validation |

Regression tests reproduced the following failures before their fixes: root-audience reply leak, owner session surviving key rotation, database outage returning 401, and HTMX redirect returning 303. Additional tests cover token refill/isolation/concurrency, trusted-proxy spoofing, login throttling, quota rollback, multi-page feeds, concurrent event publishers, and bounded cleanup.

Raw local evidence is in ignored `artifacts/verification/`: test JSON lines, vulnerability output, restore result, server logs, and screenshots. These are local artifacts, not remotely executed CI results. The code graph was consulted but its stored snapshot was stale, so test execution and source inspection supplied current evidence.

## Remaining release gates

1. Build and run the production containers on the deployment host; no Docker daemon was available locally.
2. Supply the real hostname, DNS, secrets, host/network configuration, and off-host encrypted backup destination; verify TLS externally.
3. Exercise restore from that backup destination and record the required recovery point/time objectives and measured results.
4. Configure monitoring and confirm alert delivery. The repository provides checks and guidance, not an installed monitoring service.
5. Perform broader browser and load testing appropriate to the expected community; the recorded journeys are smoke coverage, not an exhaustive accessibility, browser-compatibility, or throughput certification.
6. Put the currently untracked project under version control and run the provided CI workflow on the remote repository before a release. No commit, push, release, or deployment was performed here.

Read [operations.md](operations.md) for concrete deployment and recovery commands. Do not describe the service as publicly launched or fully RFC-complete based on these local checks.

## AWS infrastructure implementation follow-up

The `infra/` Pulumi project and release scripts now implement the isolated AWS dogfood plan. No AWS resources were applied and no DNS records changed.

- TypeScript checking passed. Both infrastructure tests passed (2 executed, zero skipped): private database/no-SSH/encrypted storage/secret boundary/DNS mode, and generated shell/Python syntax plus the EC2 user-data size limit.
- AWS-backed Pulumi previews passed using a disposable local backend: 23 creates for the foundation; 32 with the application host enabled, excluding optional Cloudflare DNS. Host preview digests were placeholders, not release artifacts. Preview JSON is in `artifacts/verification/infra/preview.json`.
- ARM64 production image built successfully through the existing Podman machine. Image ID: `1ea13f41c5135562d80bb08a9e7d5e899c3e38aa2415a98209804801287aa376`. This is a local image ID, not an ECR registry digest.
- The built image ran read-only with dropped capabilities against a fresh PostgreSQL 16 container: all five migrations applied and the container health command passed. Temporary test containers/network were removed. This supersedes the earlier lack of local container-runtime evidence; it does not prove EC2 bootstrap or RDS TLS.
- ShellCheck passed for both new release scripts; infrastructure npm audit reported zero vulnerabilities after updating the test runner.
- CI now includes infrastructure checks and an ARM64 container smoke job. These added jobs have not run remotely.

Remaining deployment evidence: committed/reviewed source and remote CI, published immutable image, actual EC2 bootstrap, Cloudflare DNS access, external TLS, confirmed notification delivery, and a real RDS recovery drill. Cloudflare proxy mode deliberately remains disabled in the optional DNS resource until forwarded-client-IP trust is implemented and tested. Read `infra/README.md` for rollout and rotation instructions.
