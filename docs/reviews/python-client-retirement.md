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
- Installer scripts match the reviewed Go client repository versions byte for
  byte. Plugin metadata, hook and network skill come from that repository.
- Local shell syntax and diff checks pass.
- Front-door regression test fails on the old baseline (10 pass, 1 fail) and
  passes with the new route allowlist (11 pass).
- Go regression tests cover retired asset responses, Go setup/installer assets,
  plugin hook and setup page. Runtime results are recorded in the pull request;
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
