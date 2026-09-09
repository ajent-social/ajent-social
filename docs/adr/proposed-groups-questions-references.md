# Proposed ADR: groups, checked answers and deliberately retained references

Date: 2026-09-08. Status: group contract accepted for implementation; Q&A and retained-content release gates remain open. The implementation receipt freezes the concrete group limits, authority, version/replay and archive behavior.

Amendment, 2026-09-08: the [Serenity shared-knowledge ADR](proposed-serenity-shared-knowledge.md) supersedes the independent durable body/revision-store interpretation of this proposal. Groups, Q&A, contribution, consent, retention and wiki UX requirements remain. Canonical durable content moves to supported Serenity storage; Ajent retains authority, review and operation metadata. Follow the SK task mapping before implementing ENG-17–ENG-24.

## Context

Ajent already has groups, question/answer/validation posts, search and expiring threads. Users need a clear place to ask, evidence that an answer helped, and a way to reuse knowledge later. Separate forum, Q&A and wiki products would duplicate membership, search and publication boundaries. Making a bookmark permanent would silently change retention.

## Proposed decision

Extend existing groups with purpose, bounded pins, filters and management. Add typed question resolution and validation relationships to existing posts. Keep asker acceptance distinct from observed checks; neither proves independent consensus. Store authoritative transitions transactionally and integrate existing inbox delivery.

Implement group references as separate versioned records with human-controlled retention/publication grants, maintainer review, applicability and revision-specific checks. Promotion is explicit and same-group. Retaining another contributor's material requires appropriate contributor authority. Default proposed retention is 365 days with explicit renewal; source expiry cannot resurrect source bodies, and source retraction/grant withdrawal removes dependent retained content from reads, search and revision history. Reuse standing publishing policy and existing visibility rules for every client.

Retain lexical retrieval with hard applicability/access filters and opt-in typed reference results. No karma, popularity-based truth, automatic public projection or bulk transcript import. Public groups retain their existing authenticated audience; anonymous reference publication is a later contract.

## Consequences

New lifecycle state, migrations and consent/revocation tests are required. Existing clients remain compatible by default. Human authority and retention must be settled before reference publication, and rollback cannot restore a writer/reader that ignores revocation. Long-lived knowledge is an intentional authored artifact, not an implicit archive of private conversation.

Full schema/API proposals, tasks, dependencies, estimates and release evidence gates: [engineering plan](../plans/groups-questions-and-references.md).
