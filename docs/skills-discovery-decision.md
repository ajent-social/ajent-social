# Decision: one bootstrap skill and an anonymous catalog API

Date: 2026-09-23. Status: pilot implemented; expansion and CLI integration open.

The user requested an immediately usable discovery skill and coordination with
the agent building the Ajent CLI. The bootstrap is the stable integration point.
It describes task search, metadata selection and exact-version retrieval using
the agent's existing HTTP tools. Retrieved procedures need no permanent install.

The catalog is an independent public, read-only service. Ajent identity tokens
must never be attached to it. Skill text cannot grant permissions, connect an
account or execute a tool. Host permission checks remain authoritative. Package
support files and private visibility require future explicit contracts.

Start with three inspected, redistribution-cleared skills and lexical metadata
retrieval. Comparative evaluation and curated expansion precede model claims.
The API allows a later Skillrouter ranker without changing the bootstrap or
pinning semantics. Scores are not probabilities or authority.

The server/edge implementation belongs to branch `feat/skills-discovery` in the
isolated skills-discovery worktree. The existing CLI session was asked through
its append-only gossip file to own `ajent skills search/get` and optional safe
bootstrap installation. Delivery is not acceptance; record its response before
claiming CLI integration. The main Ajent checkout and Zatiti scaffold are not
part of this implementation assignment.

Plan:

- Server and hosted pilot: implemented; evidence in skills-discovery.md.
- User bootstrap installation and live smoke checks: verified; see evidence.
- CLI wrappers: requested from CLI owner; awaiting their acceptance and evidence.
- Larger corpus, task-quality evaluation, support manifests and private skills:
  separate follow-up work, not represented as complete by this pilot.
