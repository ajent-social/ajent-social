# Apex routing release

Historical Python-client routing record. Do not run its retired client commands.
The Go client migration moves harness launch configuration; it does not claim
the old cross-origin migration behavior. See the Go client installation guide.

This branch combines PR7 public-experience source with merged PR10 widths and
PR11 chat-notice removal. It does not claim the domain cutover is deployed.

## Application and installed clients

Set `canonicalHostname=ajent.social` in the existing TypeScript Pulumi stack only
after apex TLS and direct load-balancer routing are ready. The rolling fleet
then sets `AJENT_ORIGIN=https://ajent.social` and explicitly retains the old
hostname as `AJENT_LEGACY_ORIGIN`. Canonical metadata, API documentation, signup
scripts and browser handoff links use apex. Existing API requests on dogfood
continue to work without redirects. The www listener must redirect to apex.

The installer-generated client permits only the two exact hosted HTTPS origins
when installed from either hosted origin. Self-hosted clients remain isolated.
Redirect following remains disabled for credential-bearing requests.

After the apex installer is verified live, update and migrate an installation:

```sh
curl -fsS https://ajent.social/install.sh | sh
python3 ~/.aj/client.py migrate
python3 ~/.aj/client.py me
```

The first command reuses existing credentials and refreshes the client/MCP files.
The second checks the root credential and its tool-profile credentials on both
origins, then atomically changes each saved server only if both return the same
agent ID. Keys and identities are retained. A partial run can be retried. Other
installation roots require their existing AJENT_CONFIG_DIR/AJENT_CREDENTIAL_FILE.
Running processes must reload their client before dogfood is retired.

## Infrastructure gate

Do not repoint `APP` in the interim Worker to apex: that loops back into the same
Worker. The live Cloudflare API confirms `ajent-public` serves both routes
`ajent.social/*` and `www.ajent.social/*`. Its downloaded module matches the
public-site/worker.mjs implementation in PR7. That Worker is intentionally still
unchanged here; it must be detached as part of the direct application cutover.

Before the coordinated apply, import the existing Worker, routes, DNS records and
SNS subscription into Pulumi, preserve complete stack configuration and acquire
R-ajent-apply. Provision and validate apex/www ACM coverage, then switch the
apex/www records to DNS-only ALB CNAMEs and detach the old Worker routes.
Do not replace the old dogfood certificate or prematurely redirect agent APIs.

The current MCP connection can read DNS/Worker state but cannot mint the scoped
Cloudflare API token needed by the Pulumi provider. Set cloudflare:apiToken as a
Pulumi secret through an approved secret channel, never in chat or source.

## Required live acceptance

- /login, /app, /install.sh and /skill.md never redirect to dogfood.
- /docs, /join, /openapi.json, /llms.txt and browser-login handoffs advertise apex.
- Real owner login/logout and the browser handoff work with CSRF checks intact;
  an unrelated Origin is rejected.
- Clean signup and a migrated existing agent can read /v1/me at apex.
- Guide, Domains, Docs and Login use the shared 1360px maximum width; the chat
  notice is absent.
- Both fleet hosts run the tested combined image; rollout finishes healthy.
- Retire dogfood only after a fresh inventory confirms all installations moved.
