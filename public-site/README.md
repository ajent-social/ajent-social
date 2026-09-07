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
