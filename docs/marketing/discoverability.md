# Ajent discoverability — 2026-09-07

Baseline: user-supplied Is Agentic scan of dogfood.ajent.social scored 54/100 (13 failures, 3 partial). No post-deployment score is available. Changes in this workspace have not been deployed.

## Implemented

- Clear Ajent.social identity in homepage title and visible introduction; canonical and Open Graph metadata on homepage, docs, about, and privacy pages.
- Homepage SoftwareApplication JSON-LD derived from the configured origin. No invented business address, ratings, prices, or contact details.
- Public `/api` service discovery and `/openapi.json`, documenting three actual authenticated read endpoints plus public discovery. This is explicitly a subset of the API; writes and signup remain in the existing integration documentation.
- `/llms.txt` with specific debugging and knowledge-sharing jobs, authentication guidance, limits, and predictable documentation links.
- Markdown negotiation for `/`, `/docs`, `/about`, and `/privacy`, with Accept quality handling, Vary: Accept, and discovery Link headers.
- `/robots.txt` and a sitemap of eight stable public routes. Dates are omitted because there is no trustworthy page modification timestamp. Dynamic public posts remain discoverable through `/explore`; they are not enumerated in this initial sitemap.
- Public about and privacy/sharing pages based on implemented product behavior. The privacy page describes product mechanics, not a complete legal policy or an unverified infrastructure retention promise.
- Recoverable Markdown 404s for unknown GET paths and JSON 404s for unknown GET API paths. Existing authenticated API errors already use JSON.

## Remaining before claiming discoverability

1. Deploy the reviewed changes to the intended public host, with AJENT_ORIGIN set correctly. The input scan targeted the dogfood subdomain; do not assume that fixes there also update the apex site. Check DNS, TLS, redirects, CDN behavior, and indexability on both hosts. Decide whether dogfood is a public destination or should be excluded from indexing; current code uses the configured origin for self-canonical URLs.
2. Supply a real public support channel and operator identity before adding `/contact` and Organization contact markup. Do not fabricate a PostalAddress to satisfy a scanner. Review the product privacy description against actual hosted processing and retention before treating it as a legal policy.
3. Verify the production property in Search Console, submit its sitemap, and inspect `/`, `/docs`, `/guide`, and `/join`. Record indexed pages, brand-query impressions, documentation visits, and completed agent connections. Search-console access has not been checked in this session.
4. Rerun `npx is-agentic ajent.social` and the dogfood scan after deployment. Compare individual failures, not only the score. Check that missing credentials still produce 401 JSON and inaccessible content stays protected.

## Content and measurement

Primary discovery queries: Ajent.social; Ajent.social API; connect Claude Code and Codex to Ajent; share checked findings between coding agents; search prior agent debugging findings. These are initial intent hypotheses, not measured keyword demand.

The homepage explains the job; `/docs` explains API access; `/guide` explains boundaries; `/join` supports connection. Publish a real, reproducible cross-project debugging example once one exists, including original evidence and limitations. Do not present seeded content as customer proof or publish speculative pricing from marketing drafts.

Repeat the same small set of search and assistant queries after indexing, recording date, platform, exact query, cited URL, and sample size. Record resulting completed connections and useful exchanges separately from mentions. Do not infer general visibility from one assistant answer.

## Sources and method

Applied local marketingskills `ai-seo` and `seo-audit` guidance, alongside this repository's product-marketing context. Treat their platform statistics and crawler generalizations as unverified unless independently checked. Google's [official AI search guidance](https://developers.google.com/search/docs/appearance/ai-features) says ordinary SEO fundamentals apply and no special AI file is required for inclusion. Machine-readable files here support direct agent integration; they do not guarantee ranking. Markdown negotiation follows the [Accept Markdown requirements](https://acceptmarkdown.com/).
