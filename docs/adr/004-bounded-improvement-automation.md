# ADR 004: Durable and bounded improvement automation

## Status

Accepted planning direction; implementation pending E30/E31. Later deployment, fleet and dashboard details remain gated plans.

## Date

2026 09 08

## Context

The private report and verification loop exists, but a local coordinator only projects reports after operator review. A laptop lock cannot coordinate production replicas. An external issue creation can succeed while its response is lost. Model suggestions and reporter prose cannot safely become unrestricted commands or publication authority.

## Decision

Use the existing Go application, background-job infrastructure and PostgreSQL for bounded production improvement work. Isolate its scheduling and deadlines from request serving, event publication and retention. Do not introduce another orchestration platform. Store durable job ownership, attempts, projection intents and sanitized audit outcomes in PostgreSQL. Claim work with expiring leases and monotonically increasing fencing generations; every state mutation checks the current generation. Never hold a database transaction over a network or model call.

A GitHub create intent is durably marked before sending. A timeout, process death or lost lease after that point becomes ambiguous: successors search for the exact marker and attach an existing issue or escalate. They never blindly repeat a possibly successful creation. Fencing database writes does not make GitHub exactly-once. Preserve the current safe projection and retain manual recovery for uncertainty. A single coordinated cutover imports the local journal and stops the old scheduler before cloud writes start.

Give a triage worker only a typed action vocabulary: classify evidence, suggest a same-repository issue match, assign a bounded priority and owner, write a private explanation, or request review. It cannot execute supplied commands, access arbitrary URLs, close reports as fixed, deploy code or automatically merge feature reports. Deterministic policy gates validated proposals. Start with shadow proposals; enable only evaluated low-risk transitions. Access, delivery-loss, ambiguous duplicates and insufficient evidence go to review with urgency, never automatic decline.

Model access is optional and bounded by configured call, token and monetary ceilings. Only approved processing destinations may receive private context; existing chat credentials alone are not permission to forward improvement reports. The default context-free path uses captured allowlisted diagnostics and deterministic rules. Provider failure or missing policy leaves reviewable work, not invented triage. Record policy/model versions and outcome codes, not prompts or report bodies in aggregate logs.

GitHub credentials are scoped to the selected repository, stored through existing production secret management and accessed through a dedicated adapter. A missing credential disables projection explicitly. Keep existing operator APIs and reporter privacy; automation is a separately provisioned operator with an immediate disable switch.

## Consequences

Production operation survives laptop sleep and instance replacement. Durable ownership reduces duplicate work, but unresolved external outcomes still need review. Narrow automatic actions deliver useful triage without turning feedback into an autonomous code-execution channel. Explicit shadow evaluation and live fault checks cost effort before rollout; they are the acceptance gate. A later deploy event can request verification, but only an affected workflow observation can resolve a report.
