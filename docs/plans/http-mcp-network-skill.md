# Portable HTTP MCP participation skill

## Context and authority

The owner requested a plan, implementation and production deployment on 2026-10-29 and explicitly approved proceeding without another approval round. The deployed HTTP connector exposes nine scoped tools; the existing installed-client skill refers to additional capabilities. A remote MCP connection does not install a platform skill automatically.

This file governs only this bounded feature; the master growth/improvement plan remains unchanged. Executor: current session, ordinary repository delivery, engineering profile. Existing GitHub self-hosted CI and the reviewed Fargate release executor remain the verification/deployment routes. Graph tools were unavailable; source inspection qualified the fallback. No new provider, installation grant, infrastructure design, OAuth scope or tool effect is needed.

## Outcome and design

One versioned, embedded SKILL.md describes when to use Ajent, identity and workspace scope, finding/reply quality, explicit inbox processing, idempotent retries, privacy and owner authority. Its tool table matches the nine HTTP tools. Serve that same content as a public skill document, MCP initialization instructions and a discoverable MCP resource. Clients that ignore resources still receive initialization instructions; manual platform skill installation remains an explicit client action.

Acceptance: canonical document is available without credentials, initialization and resource reads return the same origin-correct guidance, no extra tools/scopes are introduced, read-only clients can discover guidance, and existing authentication challenges remain intact. Tests cover protocol discovery and safety-critical participation scenarios. Production acceptance requires the exact reviewed/landed source, image digest, successful fleet refresh, healthy targets, public skill bytes/version, OAuth+CIMD discovery and MCP authentication challenge. Real provider adoption remains a separate outcome.

## E90 -- HTTP participation guidance

fidelity: executable
Acceptance: portable guidance reaches compatible clients and is verified in production without claiming automatic skill installation or universal provider compliance.

- [ ] T90.0 Reconcile scope and source  Owner: parent kind: agent stage: preflight  acc: [base7c188493, nine tools, existing skill gap and owner deployment authority recorded]
- [ ] T90.1 Implement canonical skill and discovery surfaces  Owner: parent kind: agent stage: implement  blocked-by: [T90.0]  acc: [one versioned embedded document serves HTTP and MCP initialization/resource discovery; documentation explains platform adoption and boundaries]
- [ ] T90.2 Verify protocol and participation contracts  Owner: parent kind: agent stage: verify  blocked-by: [T90.1]  acc: [targeted protocol/public-route tests and CI pass for exact candidate; scenarios cover authorized reply, no sharing authority, prompt injection, missing scope and retry key reuse]
- [ ] T90.3 Independently review T90.1 and T90.2  Owner: reviewer kind: agent stage: review  blocked-by: [T90.2]  acc: [independent reviewer records exact head/base and dispositions; accepted findings fixed and re-reviewed]
- [ ] T90.4 Guarded rebase merge  Owner: parent kind: agent stage: merge  blocked-by: [T90.3]  acc: [normal rebase merge matches reviewed head and landed tree is verified]
- [ ] T90.5 Verify landed source and release gates  Owner: parent kind: agent stage: verify-landed  blocked-by: [T90.4]  acc: [exact main checks pass, control revision verified, no active release/update and owned deployment claims acquired]
- [ ] T90.6 Deploy and verify production guidance  Owner: parent kind: agent stage: verify  blocked-by: [T90.5]  acc: [owner-authorized Fargate release exact source/image deployed, fleet and live skill verified, temporary credentials revoked and owned claims released]

## Batch and recovery

| Batch | Tasks | Implementation | Verification | Review | Delivery |
|---|---|---|---|---|---|
| HTTP-SKILL-1 | T90.0 through T90.6 | one coherent source candidate | targeted protocol and existing CI | independent exact-head reviewer | guarded merge, exact-source Fargate release and live acceptance |

An uncertain release is reconciled before retry; no duplicate deployment. Temporary personal Pulumi token and JIT cleanup run in finally, including failure. Existing unrelated worktrees and authored material are preserved. The previous runtime04 interrupted its workflow supervisor while AWS completed successfully; new release supervision must preserve uncertain outcomes and independent runtime evidence. No billing/access expansion or hand infrastructure changes.

## Evidence

Task record: `/Volumes/BuildOffload/agent-task-records/ajent-http-mcp-skill-20261029`. Base: `7c18849361c1f8b3f99eeae18865b61c4bd34f12`. Source, local/CI tests, independent review, merge and deployed/client acceptance are recorded separately. Completion status is maintained in that record so source acceptance metadata stays immutable after review.
