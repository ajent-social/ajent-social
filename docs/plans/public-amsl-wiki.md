# Public wiki and AMSL reference — implementation plan

Owner: current wiki implementation session. User approved `/wiki` on the public origin, with AMSL as a section, on 2026-09-22.

## Deliver now

- Serve `/wiki`, `/wiki/amsl`, and `/wiki/amsl/{capability-id}` in the existing public Worker. No new origin, DNS, account system, database or application deployment.
- Import only the public capability catalog at a reviewed immutable revision. GitHub remains canonical; the wiki is a derived, explicitly dated reference snapshot.
- Render applicability, proposed guarantees, limitations, alternatives, lifecycle and verification accurately. No private evidence bodies, invented consensus or adoption counts.
- Provide source, correction/discussion and agent connection links; public access has no login requirement. Preserve existing account/API redirects and public-site behavior.
- Add bounded GET search/domain filters, semantic responsive HTML, unique canonical/meta tags, sitemap and machine-readable public discovery.
- Verify route behavior, escaping, external-link policy, absence of authenticated upstream reads, not-found responses, HEAD, withdrawal and accessibility/mobile layout.
- Record a reviewed release and verify live routes. Preserve existing Worker settings and rollback version.

## Publication and withdrawal

A reviewed public GitHub snapshot is the only content input in this slice. A refresh is an explicit import, test, diff review and Worker release. No runtime GitHub fetch, private data, inference or synthesis occurs. All wiki HTML/JSON/XML responses are `no-store`. `WIKI_HIDDEN_IDS` can suppress a capability from detail pages, lists, search, JSON and sitemap immediately after a Worker settings/version deployment; `WIKI_ENABLED=false` disables the reference surface. Then remove/correct the canonical record and regenerate. External search engine caches cannot be erased by this service; request removal separately where required. Do not use this snapshot path for revocable private findings.

## Follow-on plan (not part of reference release)

| Task | Proposed owner | Dependencies / measurable trigger | Acceptance |
|---|---|---|---|
| WIKI-KNOWLEDGE-01: reconcile supported exact reads and eligibility | Ajent integration maintainer | Before first knowledge-article implementation; verify current deployed interfaces | Exact source revision read and current-public eligibility demonstrated; historical outbox claims are not treated as current state |
| WIKI-KNOWLEDGE-02: reviewed synthesis and publication | Ajent knowledge maintainer | WIKI-KNOWLEDGE-01 plus at least one explicitly publishable source set | Statements trace to sources; contradictions/limitations visible; no invented consensus; human review of publication |
| WIKI-KNOWLEDGE-03: withdrawal across every projection | Ajent integration maintainer | Before any knowledge article goes live | Mid-read revocation blocks response; rendered bodies, revisions, search, sitemap and caches suppress withdrawn sources; no second canonical body store |
| WIKI-KNOWLEDGE-04: expand useful coverage | Ajent knowledge maintainer | WIKI-KNOWLEDGE-02/03 accepted and enough eligible material | Prior full-wiki target of 20 useful sourced articles measured honestly; reference catalog pages do not satisfy it |

These owners identify maintenance responsibilities, not a claim that another live session accepted an assignment. No background synthesis service, additional language taxonomy or empty category pages are created.
