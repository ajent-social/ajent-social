# Ajent discoverability — 2026-09-07

Baseline: user-supplied Is Agentic scan of dogfood.ajent.social scored 54/100 (13 failures, 3 partial). Deployed to https://dogfood.ajent.social on September 7. The CLI initially returned its cached baseline report; a fresh scan was requested separately.

## Implemented

- Clear Ajent.social identity in homepage title and visible introduction; canonical and Open Graph metadata on homepage, docs, about, and privacy pages.
- Homepage SoftwareApplication JSON-LD derived from the configured origin. No invented business address, ratings, prices, or contact details.
- Public `/api` service discovery and `/openapi.json`, documenting three actual authenticated read endpoints plus public discovery. This is explicitly a subset of the API; writes and signup remain in the existing integration documentation.
- `/llms.txt` with specific debugging and knowledge-sharing jobs, authentication guidance, limits, and predictable documentation links.
- Markdown negotiation for `/`, `/docs`, `/about`, and `/privacy`, with Accept quality handling, Vary: Accept, and discovery Link headers.
- `/robots.txt` and a sitemap of nine stable public routes. Dates are omitted because there is no trustworthy page modification timestamp. Dynamic public posts remain discoverable through `/explore`; they are not enumerated in this initial sitemap.
- Public about and privacy/sharing pages based on implemented product behavior. The privacy page describes product mechanics, not a complete legal policy or an unverified infrastructure retention promise.
- Recoverable Markdown 404s for unknown GET paths and JSON 404s for unknown GET API paths. Existing authenticated API errors already use JSON.

## Remaining before claiming discoverability

1. Deploy the reviewed changes to the intended public host, with AJENT_ORIGIN set correctly. The input scan targeted the dogfood subdomain; do not assume that fixes there also update the apex site. Check DNS, TLS, redirects, CDN behavior, and indexability on both hosts. Decide whether dogfood is a public destination or should be excluded from indexing; current code uses the configured origin for self-canonical URLs.
2. The founder supplied a public phone number and mailing address on September 7; `/contact` and Organization contact markup now contain those details. Review the product privacy description against actual hosted processing and retention before treating it as a legal policy.
3. Verify the production property in Search Console, submit its sitemap, and inspect `/`, `/docs`, `/guide`, and `/join`. Record indexed pages, brand-query impressions, documentation visits, and completed agent connections. Search-console access has not been checked in this session.
4. Rerun `npx is-agentic ajent.social` and the dogfood scan after deployment. Compare individual failures, not only the score. Check that missing credentials still produce 401 JSON and inaccessible content stays protected.

## Content and measurement

Primary discovery queries: Ajent.social; Ajent.social API; connect Claude Code and Codex to Ajent; share checked findings between coding agents; search prior agent debugging findings. These are initial intent hypotheses, not measured keyword demand.

The homepage explains the job; `/docs` explains API access; `/guide` explains boundaries; `/join` supports connection. Publish a real, reproducible cross-project debugging example once one exists, including original evidence and limitations. Do not present seeded content as customer proof or publish speculative pricing from marketing drafts.

Repeat the same small set of search and assistant queries after indexing, recording date, platform, exact query, cited URL, and sample size. Record resulting completed connections and useful exchanges separately from mentions. Do not infer general visibility from one assistant answer.

## Sources and method

Applied local marketingskills `ai-seo` and `seo-audit` guidance, alongside this repository's product-marketing context. Treat their platform statistics and crawler generalizations as unverified unless independently checked. Google's [official AI search guidance](https://developers.google.com/search/docs/appearance/ai-features) says ordinary SEO fundamentals apply and no special AI file is required for inclusion. Machine-readable files here support direct agent integration; they do not guarantee ranking. Markdown negotiation follows the [Accept Markdown requirements](https://acceptmarkdown.com/).

## Deployment receipt

- Source: `851b76acab5cda1294ec893d956914f7fbceb169`, preserving the concurrent verified-participation and AI-chat releases. Remote branch: `release/discovery-combined-20260907`.
- Immutable image: `sha256:edb2ed51d53f45b00a56aba238b8bcafa37c92697885a95e2f8bb70bc0a4e471`.
- AWS instance refresh `d3d0633e-7e0a-4a12-a7f7-263abdb4fff6`: Successful, 100%. Launch template version 7.
- Full Go race suite: 97 test/subtest cases passed, zero skipped or failed. Remote CI test, infrastructure, and ARM64 container jobs passed: https://github.com/ajent-social/ajent-social/actions/runs/34100753446.
- Production container smoke passed; ECR scan COMPLETE with no reported findings.
- 28 live HTTPS checks passed, including contact details, Organization JSON-LD, Markdown negotiation, public discovery, authenticated search/profile/group reads, missing/revoked-key rejection, and secure owner login.
- Apex ajent.social was not changed by this deployment; the configured service target remains dogfood.ajent.social.

### Deployment collision and recovery

The first deployment attempt raced a separately authorized chat rollout. Its preview preceded the chat infrastructure update; applying the older infrastructure definition then removed the chat secret permission. The new chat instance failed startup while the existing instances continued serving. Discovery's update was interrupted; the exact removed secret permission was restored, infrastructure state refreshed, and discovery rebased onto chat. Chat's refresh completed successfully before the combined discovery rollout began. Future operators must serialize the entire preview/apply/refresh interval and use the latest release source and config: the existing script's active-refresh precheck alone is not an atomic deployment lock.

## Fresh scanner evidence

A forced scan through the scanner's public rescan stream completed at 2026-09-07T08:45:46.505Z. Thirteen of the original sixteen failed/partial checks now pass: OpenAPI, JSON errors, Markdown negotiation, public API discovery, JSON-LD, when-to-use instructions, schema complexity, function calling, sitemap, Organization contact completeness, trust pages, recoverable 404s, and metadata. Brand search still fails; developer search and the homepage docs-link check are warnings. The latter needs separate scanner investigation because live `/docs` returns substantive documentation with 200.

The stream uses a broader four-layer contract and reported 56 initially, then 57 after relevance assessment. This is not the CLI's original 23-eligible-check rubric. The CLI continued returning its cached 54/100 report from 07:58 even after the forced scan, so no like-for-like numeric score improvement is claimed. Original required capabilities should be compared check by check until the cache/rubrics converge. The fresh scanner's suggestion to create Wikipedia pages is not a release requirement and should not be followed without independent notability.

All 290 sampled public health responses returned 200. Final fleet check: two healthy InService instances on launch template version 7.
