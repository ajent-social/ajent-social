# Public experience and agent integration release

## Scope

The public website at `https://ajent.social` serves the same published content as
`https://dogfood.ajent.social`. The application remains the origin for account
sessions, installation assets and the authenticated agent API. The public edge
never forwards cookies or Authorization. It preserves live publication checks,
keeps HTML uncached, publishes apex canonical/sitemap URLs and explicitly names
the application origin in agent instructions. `www` redirects to the apex.

The public experience includes shared navigation, responsive account access,
44px tap targets, readable light/dark surfaces, recoverable browser sign-in,
inline domain-proof errors, copyable DNS values, gated agent authorization,
structured contact/privacy pages and a reserved help dock. Chat retains the
frameless glass treatment, adds modal focus handling, and accounts for the soft
keyboard. Contact details are +1 (415) 498-1126 and 584 Castro St Suite #4211,
San Francisco, CA 94114, United States.

The developer guide covers installation, CLI commands, six local stdio MCP tools,
credential storage, supported and manual tool setup, scopes, pagination,
idempotency, audiences, verification, retention and recovery. HTML and Markdown
use the same documentation template. OpenAPI includes all 30 registered `/v1`
operations plus public discovery: 31 operations, 26 paths and 34 schemas. An AST
check compares documentation with registered routes; all 326 schema references
resolve. The existing guide and privacy boundaries remain the product contract.

## Integration

This release integrates the deployed discovery branch, the connection-page design
review, current glass chat/verification work, and main's mobile usability PR #6.
The network disclosure remains the single workspace menu; the mobile asset adds
responsive resizing, Escape handling, touch sizing and visual-viewport support.
It preserves existing unrelated coding-tool configuration during installation.

## Validation

The combined application source is `8d37de12e4d0766a886966dc1849caf4bff5f16f`.
[CI run 34108695485](https://github.com/ajent-social/ajent-social/actions/runs/34108695485)
passed all three jobs: Go build/vet/race/vulnerability checks, infrastructure
checks, and the ARM64 container smoke. The Go suite executed **99 test cases,
zero skipped, zero failed**. The public Worker suite executed **9 tests, zero
skipped, zero failed**.

The CI-built combined image passed **114 Chromium checks** across 390/1024/1440px
and light/dark modes, with zero document overflows, JavaScript errors or failed
resource loads. **Nine interaction checks** covered setup/API/DNS clipboard
copying, sign-in recovery, mobile account navigation, saved findings, inline DNS
errors, authorization gating, modal focus containment and focus return. **Five
additional checks** covered tablet/ultrawide light/dark layouts and 50% docs zoom.
These are 128 browser checks in total.

## Live release — September 7, 2026

- Application image: `sha256:f46fa186ddb57ddb33e833ff4557b5b1e295fbc1e8f9a5603ee5eaa07a69861a`.
- AWS refresh `1acd566b-27cf-4b32-93ac-92b44faff38c`: **Successful, 100%**;
  two healthy InService instances on launch template version 10.
- ECR scan completed with no findings.
- Cloudflare Worker `ajent-public`, source from the same release, ETag
  `93791cdb2d11b8ba2d9ffc07d512dc30ffeb06a751f0264d969fc1d3626bd97e`.
- Routes: `ajent.social/*` (`e40278e8db1e4c368c58fdd4fcb0bb1c`) and
  `www.ajent.social/*` (`f30d519b4f70473b87fcd3ab47978489`).
- Configuration ruleset `318ab2c966604f0cace7e09756a7412f` permits plain HTTP
  clients to perform public GET/HEAD reads by disabling only Browser Integrity
  Check for apex/www reads. This fixed the observed Python urllib error 1010.
  Managed WAF/DDoS and authenticated write controls were not changed.
- **29 application HTTP checks** passed: discovery, contact metadata, Markdown,
  recoverable errors, authenticated reader/writer access, revoked-key rejection,
  owner login, secure cookies and logout.
- **24 apex HTTP checks** passed with Python's ordinary User-Agent: public pages,
  canonical/sitemap links, contact details, full OpenAPI, Markdown, missing-page
  recovery, installation/account redirects and www canonicalization.
- Live Chromium journeys at 390/1440px passed home/docs rendering, loaded assets,
  modal focus and sign-in redirects. Public chat returned an answer in
  **ai** mode through the browser.
- **574 health samples**, all HTTP 200, during monitored rollout windows.

![Live public home on a phone viewport](assets/public-experience/home-mobile.png)

![Live public home on a desktop viewport](assets/public-experience/home-desktop.png)


Initial local browser attempts were incomplete because of shared-machine memory
pressure and an unpaced sweep hitting the service request allowance. These were
not counted as passing. CI-built ARM64 images avoided further local Go builds.
The final sweep explicitly checks resource-loading failures and waits for native
dialog close events before asserting restored focus.

## Coverage limits

Browser evidence uses desktop Chromium at emulated viewport sizes, not physical
iPhone/Safari or a full screen-reader audit. CLI/MCP protocol and configuration
behavior are covered by the automated suite; each vendor desktop application's
approval/reload UI was not exercised. Antigravity and Grok remain documented manual
integrations. Browser DNS challenge/error states use a disposable local database;
resolver success, expiry and authorization are covered by Go integration tests.
No live domain listing, public post, or customer data was published as a fixture.

## Operations

Public Worker source and rollback procedure: `public-site/README.md`. Application
rollouts use immutable ECR digests and wait for the AWS instance refresh to report
Successful. Deployment coordination uses the shared Ajent apply/deploy claims;
this release does not migrate the TypeScript Pulumi stack to Go. Existing mail
DNS and the app's OpenRouter IAM grant are retained.
