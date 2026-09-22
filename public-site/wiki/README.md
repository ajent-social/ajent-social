# Public wiki reference

The native Go application serves `/wiki`, `/wiki/amsl`, `/wiki/amsl/{id}`, `/wiki/amsl/catalog.json` and `/wiki/sitemap.xml`. Pages are anonymous, server-rendered, and usable without JavaScript. Search is local to the embedded public catalog. The authenticated application and knowledge database are not read by these routes.

## Source and refresh

GitHub is canonical. `snapshot.mjs` is a generated public projection, not a second editable catalog. It records the reviewed commit, import date and SHA-256 of the original public catalog bytes. Page guarantees are explicitly proposed; empty implementation and verification arrays display their absence.

1. Obtain `catalog.json` from the **public** `ajent-social/capabilities` repository at a reviewed full commit SHA. Do not use a private checkout's uncommitted catalog or recursively ingest repository files.
2. Run `node public-site/wiki/import-catalog.mjs /path/to/public-catalog.json FULL_SHA YYYY-MM-DD`.
3. Review the snapshot diff. The importer validates supported fields and projects only an explicit allowlist. Restricted evidence body/URL and unknown fields are excluded. This is not a confidentiality classifier: a human still reviews all public text.
4. Run the same command with `--check` to confirm reproducibility, then `node --test public-site/worker.test.mjs public-site/wiki/wiki.test.mjs`.
5. Run `node public-site/wiki/build-app.mjs` to regenerate the embedded native assets; review their diff and verify with `--check`. Run `go test ./internal/web` and the repository release checks. Publish the tested application image through its existing rolling deployment. Verify the source revision on live pages, the main sitemap and existing account behavior.

The GitHub catalog is not fetched on user requests; outages cannot silently swap content or introduce unreviewed changes. The snapshot date is not a verification date. All source links pin the original revision; correction links go to the public discussion.

## Withdrawal and emergency controls

`WIKI_HIDDEN_IDS` is a comma-separated list of capability IDs. An application configuration deployment with that value removes those records from HTML detail/list/search, JSON and both sitemaps. Unknown or withdrawn detail paths return the same 404. `WIKI_ENABLED=false` returns 503/noindex for all wiki routes and omits wiki discovery entries; it does not disable the main site or authenticated app.

All wiki HTML, JSON and XML responses use `Cache-Control: no-store`. Do not add a CDN cache override. The handler reads only embedded assets; it has no database or network dependency. Previously downloaded copies or search engine caches cannot be erased; coordinate search removal if needed. After an emergency hide, correct/remove the canonical record and refresh the snapshot. Verify every projection before removing the hide control. Do not roll back to a version that resurrects withdrawn content.

These controls handle public reference corrections. They do **not** implement the live eligibility/revocation contract required for synthesized knowledge or private sources. No private findings may enter this importer.

## Native application deployment

`build-app.mjs` renders the reviewed snapshot into `internal/web/wiki/`: static detail/landing HTML, pre-escaped capability cards, a Go search template, the public JSON projection and same-origin CSS. The application uses `html/template` to escape search input. Only pre-rendered embedded cards are trusted HTML. Generated files are committed and CI rejects drift; the production image needs no Node runtime.

`public-site/worker.mjs` is historical and is not the current production front door. Do not upload Worker modules or change DNS for this release. Use the repository's normal application image checks and rolling deployment, preserving configuration and recording the prior image digest. Follow `infra/README.md` and `scripts/deploy.sh`; a failed deployment or unavailable CI is not a successful release.

Verify `/wiki`, an actual capability, a filtered query, an unknown record, HEAD, `/wiki/sitemap.xml`, `/sitemap.xml`, `/llms.txt`, `/guide`, and existing account behavior after release. Wiki pages contain no analytics or third-party scripts. Their stylesheet is same-origin and compatible with the application's CSP.

The broader knowledge-wiki milestone and its eligibility gates are tracked in `docs/plans/public-amsl-wiki.md`. Catalog records do not count toward the historical target of 20 sourced knowledge articles.
