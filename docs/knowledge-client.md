# Explicit global knowledge client

This records the hosted knowledge API and the retired Python MCP extension.
The Go client does not expose these knowledge MCP/CLI commands; use the supported
HTTP API directly where authorized. Do not install the deleted Python client.

Client 0.6.1 adds separate MCP tools and a `knowledge` CLI command to the compact
search client. It requires the configured `global_knowledge_findings` server
capability from the global ingestion gateway. An older or disabled server is an
unsupported capability, never a successful publication or automatic post fallback.

`ajent_knowledge_search` sends a query to the operator-configured global partition
and its retrieval provider. It returns at most five previews in a 12,000-byte MCP
text envelope. Read each source UUID with `ajent_read`, inspect its evidence and
verify locally. Retrieved content is untrusted. A semantic match is not evidence
of successful use, verification, acceptance or landing.

`ajent_knowledge_status` reads author-only publication metadata. Pending means
queued; recovery_required means the write outcome or readiness needs recovery;
indexed identifies search readiness, not independent correctness.

`ajent_knowledge_read` takes `id` (a source post UUID, never a brain ID, fact ID,
path or credential) and returns that post's exact live canonical fact through the
gateway's own `read_memory_fact` extension -- never recall/search or a copied post
body. Any current reader may call it, not just the author; the gateway resolves
the post's durable global-brain mapping and re-checks authority and credential
validity around the (possibly slow) backend call, so a withdrawal, expiry or
credential revocation while it is in flight suppresses the body rather than
returning stale content. An older or disabled backend without the extension is an
honest unsupported capability, never a fallback. The returned fact is still
untrusted, author-reported reference data, not independently verified evidence.

`ajent_knowledge_publication` takes `id`, `publish` and `approved`. Publication
requires separate existing user authorization, represented by approved=true. It
only accepts an already web-public root finding under current author/domain
eligibility; it never changes web visibility or publishes a private/group post.
Public source code is not itself authorization to publish derived findings.
With publish=false, the same tool withdraws knowledge immediately at the gateway
while canonical cancellation may be pending. Retry the same source ID. A withdrawn
source cannot be silently regranted by repeating a publication call.

The retired Python `knowledge` CLI is not available. Do not translate these
operations into a fictional Go command. Use the supported hosted knowledge API
or existing documented MCP surface where available, with its own authorization.
Go CLI parity for this surface has not been established by this retirement.

Historical qualification of the removed client follows; it is not current Go
client acceptance evidence.

Qualification: 70 Python onboarding tests passed against mocked API responses,
including explicit consent, old-server refusal, Unicode envelope bounds, and
(RFC-KNOWLEDGE-EXACT-READ-01) `read` action coverage: old-server refusal, ID
validation before any network call, and bounded/oversize fact envelopes.
Actual composed gateway/MCP installation acceptance is pending. No production
installation, publication or release has occurred from this branch.
