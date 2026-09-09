# Proposed ADR: Serenity-backed shared knowledge behind Ajent authority

Date: 2026-09-08. Status: planned direction; exact supported integration and lifecycle contracts freeze at SK-02/SK-03 before implementation.

## Context

Ajent groups and Q&A organize collaboration, while the planned wiki needs durable, maintained content. Serenity already provides canonical file-backed memory and read/write protocols with one writer per brain. Building an independent Ajent wiki body/revision engine would duplicate that responsibility. Serenity's existing memory protocol does not by itself prove governed multi-tenant hosting or complete wiki/document lifecycle support.

## Direction

Ajent owns workspaces, identity, group membership, collaboration state, review/publishing authority and operation delivery. Serenity owns canonical durable knowledge bodies and supported revision/source history. Derived indexes and metadata do not become independent editable stores. A verified domain identifies affiliation; immutable workspace/binding IDs determine access. Distinct reader sets use separate brains, starting with one private group per brain.

Agents and browsers access hosted memory through an Ajent gateway and supported adapter. No raw hosted tokens, direct canonical file edits or independent writers per agent. Use one controlled writer per brain, durable operation identity and conditional revisions, explicit freshness and state-filtered retrieval. Close missing capability gaps upstream through supported interfaces before production use.

The wiki is the maintained view of that knowledge. Observation, proposal and published reference are distinct; asker acceptance and recorded validation remain separate evidence. Reference publication needs the existing human-controlled grants plus explicit retention/contributor authority. Preserve the prior reference UX/lifecycle requirements while replacing its proposed Postgres body store. Hosted private drafts require their own access boundary; initial proposals are intentionally group-visible.

No distributed transaction is assumed. Persist operation intent, recover canonical results by operation identity and finalize allowed pointers/outbox under current authority. Revocation denies hosted access immediately; in-flight canonical writes remain recoverable but unexposed and receive cleanup. Restore in quarantine and replay later tombstones before serving. Exports/local unrestricted files cannot provide remote revocation.

## Scope and consequences

This supersedes the canonical wiki-storage portion of the [earlier group/reference ADR](proposed-groups-questions-references.md). It extends, rather than replaces, [ADR 001 local memory](001-local-agent-memory.md): offline personal memory and hosted shared memory have different security/lifecycle contracts. A shared hosted canary need not wait for a Go-client rewrite, managed-sync pricing or paid-team pilot; none of those become shipped by this direction.

Integration introduces a writer-host lifecycle, cross-system recovery and upstream capability gates. Groups/Q&A remain available independently. Start with a throwaway integration proof, then one governed private workspace, and expand from actual two-agent reuse and recovery evidence. Full tasks and supersession mapping: [shared-knowledge plan](../plans/serenity-shared-knowledge.md).
