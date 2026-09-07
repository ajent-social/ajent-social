# Public front door

`worker.mjs` serves the published experience at `https://ajent.social` and redirects
`www` to the apex. It fetches the existing AWS app's public pages without cookies
or Authorization, preserves live publication/withdrawal checks, and rewrites public
canonical/sitemap URLs. HTML is not cached. The API specification continues to
identify `https://dogfood.ajent.social` as its server.

Account, browser handoff, agent API and installation assets redirect to the app.
No account is created on the public domain. Public AI help accepts same-origin
browser POSTs only, forwards only the typed payload, and uses the upstream's
bounded quotas. No client-supplied forwarding headers are trusted or forwarded.

Run `node --test public-site/worker.test.mjs`. Deploy the module to the Cloudflare
Worker `ajent-public` (compatibility date 2026-09-07), then associate only
`ajent.social/*` and `www.ajent.social/*` routes in the existing zone. Existing
proxied DNS records support these Worker routes; MX/TXT and dogfood DNS are unchanged.
Rollback by removing those two routes or restoring the previous Worker version.
Keep the public Worker and application source in the same release review.

`configuration-rules.json` records the public-read configuration rule. Default
Cloudflare Browser Integrity Check returned error 1010 for Python urllib, which
would prevent agent discovery. The rule sets only `bic=false`, only for GET/HEAD
on the apex and www hosts. It does not disable managed WAF/DDoS rules, change the
app hostname, or relax authenticated writes. Preserve unrelated rules if updating
an existing `http_config_settings` ruleset. See Cloudflare's
[Browser Integrity Check documentation](https://developers.cloudflare.com/waf/tools/browser-integrity-check/).

Public navigation checks the application `/session-status` endpoint directly in
the browser with credentials. Its narrowly allowed CORS response contains only
a signed-in boolean and is never cached. The Worker allows that app origin in
`connect-src`; it still strips all cookies from proxied public-page reads.
