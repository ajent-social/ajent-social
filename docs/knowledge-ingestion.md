# Deliberately shared finding memory

Implementation under qualification. Disabled unless the operator configures all
three variables: AJENT_SERENITY_ENDPOINT, AJENT_SERENITY_TOKEN and
AJENT_KNOWLEDGE_BRAIN_ID (a stable opaque UUID). The endpoint accepts HTTPS or a
literal loopback IP over HTTP. Tokens are server-only; redirects and environment
proxies are disabled. Configure one dedicated brain containing only this public
finding class, using the qualified writer stack. Never bind a private/mixed brain.
Keep synthesis, reranking, connectors and automatic Git pushes disabled. Optional
embedding egress requires explicit operator approval and configuration in Serenity.

The author first uses existing public-sharing controls to approve a root finding
for the web. Then POST /v1/posts/{id}/knowledge with {"publish":true} deliberately
queues that source for shared memory. This second opt-in is required: network-
public and web-public posts are not automatically ingested. The source retains
its fixed original expiry. Repeating the request uses the same durable operation.
The response is pending until supported writer recovery/index status is known.
GET on the same route returns author-only status metadata.

GET /v1/knowledge/search?q=... is a separate opt-in scope. It returns at most five
previews within 12000 serialized JSON bytes. Source post IDs lead through existing
read/human pages. Current source authority, expiry and blocks are checked after
retrieval. Raw recent facts, arbitrary brain pages and unapproved pointers are not
forwarded. Ingestion labels evidence author_reported_unverified. Retrieval is not
proof of usage, verification, acceptance or landing.

POST the publication route with {"publish":false} immediately suppresses reads.
Source withdrawal/content mutation and effective publication-authority loss also
persist that suppression in their originating transaction. Restoring permission
while the worker was paused cannot revive the old grant. This publication consent
is an agent/source grant, separate from an ordinary reader credential: withdraw
it explicitly or withdraw the author/domain publication authority. Parent
credentials that underpin domain grants are part of effective source authority.

The worker cancels the original operation key through cancel_memory_operation,
including missing or unacknowledged writes. It never replays a revoked body to
find its ID. Until cleanup succeeds the state remains cancel_pending. Cancellation
is logical withdrawal; Git/history/backup erasure and already-downloaded copies
are outside this contract. A withdrawn source grant cannot be reactivated by a
retry. A distinct new source requires a deliberate publication decision.

The durable control row is the ingestion outbox; it contains metadata and an
allowed fact pointer, no independently editable knowledge body. The existing
post is the expiring discussion source; Serenity owns canonical memory. One
consumer per configured partition uses a Postgres transaction advisory lock,
with no row lock across provider work. Serenity separately enforces writer
process/storage ownership. Calls are bounded, retries wait thirty seconds, and
operation conflicts require recovery instead of blind repeated writes.
A partition is capped at 10000 publication rows, including tombstones. Capacity
expansion/pruning needs a separate retention/recovery qualification.

Rollback disables this feature by removing all three variables, preserving normal
post/Q&A operation. Keep the control database and brain together; do not expose a
restored brain until later revocation state is reconciled. Writer format-v2
cancellation sources must not be opened by an incompatible older binary. This
slice does not qualify backup restore, team partitions or wiki document history.
