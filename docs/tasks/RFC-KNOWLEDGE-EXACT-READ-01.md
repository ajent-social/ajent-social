# RFC-KNOWLEDGE-EXACT-READ-01 — authorized canonical observation read

Owner: hq-rfc_knowledge_read/Sonnet. Root-authorized local implementation and tests;
no push/PR/merge/deploy/HQ commits until review. Branch from local qualified
composition fb0a12f; Serenity read dependency draftPR224 at57166d6. No migration.
Own isolated ajent-knowledge-read worktree and outbox/knowledge-read only.

Provide the smallest end-to-end gateway/client path to explicitly read a published
observation's exact live canonical fact through supported read_memory_fact. Use an
existing source-post ID at the Ajent boundary; resolve its brain/fact mapping solely
server-side. Do not accept caller-selected brain IDs, URLs, paths or credentials.
No logical wiki page/revision CAS, historical withdrawn read or second content store.

Before editing, inspect current API/client naming, capability negotiation and
publication authorization. Record the chosen minimal route/tool shape in your STATUS.
Keep existing post read behavior. A missing/older Serenity extension must produce an
honest unsupported/unavailable result, never recall/search or copied-post fallback.
Capability advertisement must not promise exact read when unsupported.

Require authenticated actor, current credential and parent/scopes, live publication,
source eligibility and can_read_post BEFORE the canonical RPC, and recheck after it.
Actor-access removal, source expiry/delete, credential revocation and publication
withdrawal during a delayed RPC must suppress the body. Call only the configured
global brain for this slice; reject group/private observations. Match returned exact
opaque fact ID to the durable publication mapping, require content_untrusted and
validate response schema/size. Never log content or credentials on provider errors.
Bound the actual outgoing gateway/MCP envelope, with a clear size failure rather
than truncating fact content. No DB row/authority locks across RPC.

Tests: missing/forged/mismatched fact IDs, private-source rejection before RPC,
older backend missing extension, withdrawal and actor credential revoke during RPC,
malformed/oversize backend output, canonical body/provenance/expiry unchanged on
success. Actual process proof against pinned Serenity57166d6 and a freshly built
clean gateway/client composition, with explicit synthetic identity limits. Existing
suite and installer behavior remain intact. Full heavy checks require load≤10 and
won shared R-build-lease (exact SHA release); use BuildOffload caches/temp and own
unique DB on127.0.0.1:55439. Never background-wait for a lease. No API-key-billed
Claude, real gateway credentials or paid requests; no descendants. Read Go and
honest-verdicts skills. Root handles Ajent inbox. Reread any REVIEW.md before final.
