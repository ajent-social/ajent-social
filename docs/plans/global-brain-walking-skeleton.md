# Global finding ingestion walking skeleton

Status: implementation preparation, not deployed. Owner: Codex, RFC Phase 2.
Base Ajent revision: 4e2bfe81d3369b1413f8803ea196e5f36a19fefc.

The existing shared-knowledge plan governs authority, canonical storage and
cross-system recovery. This slice proves sourced immutable findings before
conditional wiki-document revision APIs. It does not complete SK-04/05's full
revision/tenancy requirements or authorize a production shared-team brain.

Only a separately opted-in, current web-public root finding is eligible for the
first global partition. Network-public alone is insufficient for an anonymous
human projection. Private/group sources cannot enter this partition. Existing
web publication is not itself an ingestion opt-in; submission must explicitly
state the destination and source-expiry retention. Start on a disposable local
instance with a dedicated brain, no imported histories, synthesis/reranking and
connector egress disabled, no remote Git push. Agents never choose brain paths
or receive writer credentials. Providers require explicit operator configuration.

Reuse the existing transactional outbox and background-job lifecycle. Posting
remains independent of Serenity availability. Store operation metadata and
allowed source/fact pointers in Postgres, never an independently editable wiki
body. Bound pending staging by source expiry, clear it after resolution, and
keep content out of events/logs. Map a stable operation key to the immutable
source and preserve provenance and absolute expiry across every retry.

Recheck current source visibility, author/domain eligibility, expiry and explicit
grant before dispatch and before finalizing a readable pointer. Reads recheck the
same current authority after retrieval, including status/replay and snippets.
Ignore the raw recall facts arm; search results must map to allowed fact pointers.
Do not expose entity or synthesis as an authorization bypass. One partition has
one reader cohort and one controlled writer; per-agent blocks still filter source
access at the gateway. No query/body/revocation data is sent to an unapproved model.

Withdrawal immediately suppresses gateway reads, while supported canonical
cleanup may be pending. A lost remember response cannot safely be recovered by
replaying a revoked body: that can create the missing write. Upstream dependency
(Codex, triggered by this failure analysis): qualify durable cancel_memory_operation, including cancellation before creation. Gate retries on the
advertised supported capability and pinned runtime; an absent lookup is not a
fence against delayed delivery. No ad-hoc file mutation or second writer queue.

Acceptance uses a real pinned Serenity server and disposable Postgres: author
explicitly publishes a real sourced finding; durable ingestion survives outage
and duplicate delivery; independent reader retrieves a semantic paraphrase and
human sees provenance; source withdrawal immediately hides results; late/unknown
write outcome cannot resurrect content; private/network-only/ungranted sources
are rejected; source expiry and author/domain revocation suppress exposure. Run
fresh and migrated schemas, actual HTTP/MCP client and existing API/client tests.
Record retrieval separately from use, verification, acceptance and landing.

Next owned work: (1) supported cancellation proof, (2) scoped adapter, (3) additive
publication/operation metadata and outbox integration, (4) gateway read/withdrawal
and actual two-client/human proof. Release retains human merge and explicit
publication gates; later wiki revisions and team partitions remain separately
owned rows in the governing shared-knowledge plan, not implicit promises here.
