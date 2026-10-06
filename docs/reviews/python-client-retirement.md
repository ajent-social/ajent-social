# Python client retirement

## Scope

The hosted service no longer embeds or distributes the Python Ajent client.
Delete its implementation, client-only Python tests and plugin helper. Serve
reviewed Go shell/PowerShell installers and Go setup instructions instead.
`/join.py` and `/integration.json` remain explicit HTTP 410 tombstones rather
than downloads or silent replacements. The public front-door route allowlist
includes both modern installers and setup documents.

Preserve server HTTP enrollment/link coverage, unrelated operational and
infrastructure Python tools, existing account defaults, and Git history. The
Go client repository retains only legacy-install recognition and migration
fixtures. Its implementation is Go.

## Evidence

- Service source baseline: `1853cf7`.
- Installer scripts started from client revision fd70cf3 and were hardened during independent service review: both require independent OpenSSL signature verification before installation, and new-install routes remain unavailable until an Ed25519 public key is configured. Plugin metadata, hook and network skill come from the client repository.
- Local shell syntax and diff checks pass.
- Front-door regression test fails on the old baseline (10 pass, 1 fail) and
  passes with the new route allowlist (11 pass).
- Go regression tests cover retired asset responses, Go setup/installer assets,
  plugin hook and setup page. A real Go-client enrollment/link test is enabled
  in CI with a binary built from pinned client revision `fd70cf3`; it checks
  approval, preserved credential identity, private project settings and login.
  Runtime results are recorded in the pull request;
  source inspection is not a Go test pass.
- Local installation removal deleted 20 exact Python implementation/cache/backup
  files. All 61 existing credential/profile files were byte-identical afterward,
  and the installed Go MCP server initializes and lists 18 tools. No active
  Python client process was found in the process check.

## Release and deployment boundaries

This source removal does not publish signed Go release assets or deploy the
service/Cloudflare front door. No published Go release was available during the
inspection. Until a verified release is available, the reviewed installer fails
closed; it must not fall back to downloading Python or bypass signature checks.

The original service checkout contains unrelated uncommitted work and is
preserved. Superseded checkouts and Git history may still contain historical
Python source; they are not current installation payloads. Verify the exact
service and front-door revisions after rollout before claiming live download
routes are retired. Existing installations on unreachable machines remain
unverified. Do not restore deleted client files from old backups.

## Release gate and CI access

This source change does not commission or publish the Go client's first trusted
release. The client plan's T5.0/T5.3b signing-key and distribution work is still a
release prerequisite. With `AJENT_RELEASE_PUBLIC_KEY` unset, `/install.sh` and
`/install.ps1` return 503 with no executable payload; `/join` offers existing-client
setup and explains that new installations are unavailable. Agent setup instructions
preserve the same gate. Invalid/non-Ed25519 keys reject service startup.

Only after an owner-approved, publicly accessible signed release exists, configure all three release inputs: `AJENT_RELEASE_PUBLIC_KEY` with its Ed25519 PUBLIC KEY PEM matching the Go client’s trusted release key, `AJENT_RELEASE_DOWNLOAD_BASE` with the public HTTPS asset base (assets are under `/vVERSION/`), and `AJENT_RELEASE_VERSION` with the pinned version without `v`. Partial or invalid configuration fails startup. The current private client repository has no published releases, so its unauthenticated release URL is not an installation channel. Configuring a base URL is not evidence that artifacts are publicly available; verify access and signatures before production activation. Both installers require OpenSSL 3 and fail closed if verification
is unavailable or fails. This variable contains a public key only; never put a signing
private key in the service. No production activation is performed by this PR.

The pinned private-client checkout uses `AJENT_CLIENT_READ_TOKEN`, an Actions secret
with Contents: read access only to `ajent-social/ajent`. Client repository policy
rejects deploy keys; that policy was preserved. Never substitute the owner’s broad
personal CLI token. Source/CI/merge receipts are recorded in the PR; passing service
checks does not prove that public release artifacts or signing keys exist.

## Local verification prerequisites

The ordinary zero-skips suite needs a Go client binary from pinned client revision
`fd70cf3025c4477a778511e5a21815c4cf0ec9a4`. Use an isolated client checkout and
artifact/cache directories on the approved build volume. After checking shared
load and acquiring the build lease, set `AJENT_CLIENT_SOURCE` to that checkout and
`AJENT_GO_CLIENT_TEST_BINARY` to an absolute output path, then run:

```sh
test "$(git -C "$AJENT_CLIENT_SOURCE" rev-parse HEAD)" = fd70cf3025c4477a778511e5a21815c4cf0ec9a4
go -C "$AJENT_CLIENT_SOURCE" build -trimpath -o "$AJENT_GO_CLIENT_TEST_BINARY" ./cmd/ajent
export AJENT_GO_CLIENT_TEST_BINARY
# AJENT_TEST_DSN must identify a disposable PostgreSQL database.
scripts/test.sh
```

Run the separate DNS browser suite with `scripts/test-dns-browser.sh`, using a
fresh database as documented in `docs/testing/dns-browser.md`. Missing external
fixtures are a prerequisite failure, not permission to weaken the zero-skips gate.
Release the shared lease immediately after local verification. CI uses the existing
self-hosted runners and the exact same pinned client source.
