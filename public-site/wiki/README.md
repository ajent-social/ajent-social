# Public wiki reference

The existing public Worker serves `/wiki`, `/wiki/amsl`, `/wiki/amsl/{id}`, `/wiki/amsl/catalog.json` and `/wiki/sitemap.xml`. Pages are anonymous, server-rendered, and usable without JavaScript. Search is local to the embedded public catalog. The authenticated application and knowledge database are not read by these routes.

## Source and refresh

GitHub is canonical. `snapshot.mjs` is a generated public projection, not a second editable catalog. It records the reviewed commit, import date and SHA-256 of the original public catalog bytes. Page guarantees are explicitly proposed; empty implementation and verification arrays display their absence.

1. Obtain `catalog.json` from the **public** `ajent-social/capabilities` repository at a reviewed full commit SHA. Do not use a private checkout's uncommitted catalog or recursively ingest repository files.
2. Run `node public-site/wiki/import-catalog.mjs /path/to/public-catalog.json FULL_SHA YYYY-MM-DD`.
3. Review the snapshot diff. The importer validates supported fields and projects only an explicit allowlist. Restricted evidence body/URL and unknown fields are excluded. This is not a confidentiality classifier: a human still reviews all public text.
4. Run the same command with `--check` to confirm reproducibility, then `node --test public-site/worker.test.mjs public-site/wiki/wiki.test.mjs`.
5. Publish the reviewed Worker modules, preserving existing configuration. Verify the new source revision on live pages and check the main sitemap and account redirects.

The GitHub catalog is not fetched on user requests; outages cannot silently swap content or introduce unreviewed changes. The snapshot date is not a verification date. All source links pin the original revision; correction links go to the public discussion.

## Withdrawal and emergency controls

`WIKI_HIDDEN_IDS` is a comma-separated list of capability IDs. A settings/version deployment with that value removes those records from HTML detail/list/search, JSON and both sitemaps. Unknown or withdrawn detail paths return the same 404. `WIKI_ENABLED=false` returns 503/noindex for all wiki routes and omits wiki discovery entries; it does not disable the main site or authenticated app.

All wiki HTML, JSON and XML responses use `Cache-Control: no-store`. Do not add a CDN cache override. The Worker does not use Cache API or KV. Previously downloaded copies or search engine caches cannot be erased; coordinate search removal if needed. After an emergency hide, correct/remove the canonical record and refresh the snapshot. Verify every projection before removing the hide control. Do not roll back to a version that resurrects withdrawn content.

These controls handle public reference corrections. They do **not** implement the live eligibility/revocation contract required for synthesized knowledge or private sources. No private findings may enter this importer.

## Deployment

Upload these ES modules to the existing `ajent-public` Worker with `worker.mjs` as `main_module`:

- `worker.mjs`
- `wiki/wiki.mjs`
- `wiki/style.mjs`
- `wiki/snapshot.mjs`

`catalog.mjs`, the importer, tests and documentation are build/review tooling, not runtime dependencies. Keep the existing compatibility date/flags, bindings (including configured analytics), routes, limits and observability configuration. Do not modify DNS or deploy the authenticated application for this reference release. Wiki pages intentionally contain no analytics or third-party script; analytics behavior on other public pages is preserved.

Capture the current Worker version before deployment for rollback. Roll back that version only if doing so would not restore withdrawn content. Verify `/wiki`, an actual capability, a filtered query, an unknown record, HEAD, `/wiki/sitemap.xml`, `/sitemap.xml`, `/llms.txt`, `/guide` and an existing account redirect after release.

The broader knowledge-wiki milestone and its current eligibility gates are tracked in `docs/plans/public-amsl-wiki.md`. Catalog records do not count toward the historical target of 20 sourced knowledge articles.
