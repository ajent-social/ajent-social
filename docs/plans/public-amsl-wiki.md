# Public wiki and AMSL reference — implementation plan

Owner: current wiki implementation session. User approved `/wiki` on the public origin, with AMSL as a section, on 2026-09-22.

## Deliver now

- Serve `/wiki`, `/wiki/amsl`, and `/wiki/amsl/{capability-id}` in the native Go application. No new origin, DNS, account system or database. Ship through the existing tested application image and rolling release.
- Import only the public capability catalog at a reviewed immutable revision. GitHub remains canonical; the wiki is a derived, explicitly dated reference snapshot.
- Render applicability, proposed guarantees, limitations, alternatives, lifecycle and verification accurately. No private evidence bodies, invented consensus or adoption counts.
- Provide source, correction/discussion and agent connection links; public access has no login requirement. Preserve existing account/API routes and public-site behavior.
- Add bounded GET search/domain filters, semantic responsive HTML, unique canonical/meta tags, sitemap and machine-readable public discovery.
- Verify route behavior, escaping, external-link policy, absence of authenticated upstream reads, not-found responses, HEAD, withdrawal and accessibility/mobile layout.
- Record a reviewed release and verify live routes. Preserve existing application settings and record the prior image for rollback.

## Publication and withdrawal

A reviewed public GitHub snapshot is the only content input in this slice. A refresh is an explicit import, test, diff review and application release. No runtime GitHub fetch, private data, inference or synthesis occurs. All wiki HTML/JSON/XML responses are `no-store`. `WIKI_HIDDEN_IDS` can suppress a capability from detail pages, lists, search, JSON and sitemap immediately after an application configuration deployment; `WIKI_ENABLED=false` disables the reference surface. Then remove/correct the canonical record and regenerate. External search engine caches cannot be erased by this service; request removal separately where required. Do not use this snapshot path for revocable private findings.

## Follow-on plan (not part of reference release)

| Task | Proposed owner | Dependencies / measurable trigger | Acceptance |
|---|---|---|---|
| WIKI-KNOWLEDGE-01: reconcile supported exact reads and eligibility | Ajent integration maintainer | Before first knowledge-article implementation; verify current deployed interfaces | Exact source revision read and current-public eligibility demonstrated; historical outbox claims are not treated as current state |
| WIKI-KNOWLEDGE-02: reviewed synthesis and publication | Ajent knowledge maintainer | WIKI-KNOWLEDGE-01 plus at least one explicitly publishable source set | Statements trace to sources; contradictions/limitations visible; no invented consensus; human review of publication |
| WIKI-KNOWLEDGE-03: withdrawal across every projection | Ajent integration maintainer | Before any knowledge article goes live | Mid-read revocation blocks response; rendered bodies, revisions, search, sitemap and caches suppress withdrawn sources; no second canonical body store |
| WIKI-KNOWLEDGE-04: expand useful coverage | Ajent knowledge maintainer | WIKI-KNOWLEDGE-02/03 accepted and enough eligible material | Prior full-wiki target of 20 useful sourced articles measured honestly; reference catalog pages do not satisfy it |

These owners identify maintenance responsibilities, not a claim that another live session accepted an assignment. No background synthesis service, additional language taxonomy or empty category pages are created.

## Verified hosting correction

On 2026-09-22, live DNS and `infra/README.md` confirmed that the apex serves directly from the AWS load balancer. The historical public Worker is not the current front door. An attempted Worker-only release was restored to its prior version after the live route check failed. This implementation embeds the reviewed static projection in the native application; it does not re-enable Cloudflare proxying or change DNS.

## Release gates observed on 2026-09-22

The reference implementation is not live. Local Go web tests, `go vet`, renderer tests, deterministic assets, desktop/mobile layout and keyboard entry have passed. An earlier infrastructure scan found GO-2026-6348 in gRPC v1.82.1. This branch updates to the reported fixed v1.83.1, and all four CI jobs, including the infrastructure vulnerability scan, passed at commit `1bde08c`. Current production stack configuration has no `goMigrationRevision`, so `scripts/deploy.sh` correctly refuses an image change until the separately required no-op infrastructure migration is completed.

| Task | Proposed owner | Trigger / dependencies | Acceptance |
|---|---|---|---|
| WIKI-RELEASE-01: clear infrastructure security scan — complete at `1bde08c` | Infrastructure maintainer | Earlier scan found a gRPC issue | gRPC v1.83.1 fix is included; all four CI jobs passed at this commit |
| WIKI-RELEASE-02: complete established Go migration gate | Deployment operator with architectural review | WIKI-RELEASE-01; authenticated CI runner and deployment lease; existing migration contract in `infra/README.md` | Matching TypeScript/Go no-change receipts, saved-plan no-op apply, migration revision recorded by the deployment script |
| WIKI-RELEASE-03: merge and release native wiki | Merge owner / deployment operator | Required CI, merge review, WIKI-RELEASE-02 | Tested immutable image released; live wiki, search, details, discovery, account routes and rollback verified |

Do not set the migration marker manually, re-enable the historical Worker, change DNS, or declare publication based on a successful upload alone.
