-- Metadata-only private group -> opaque brain binding. No canonical body store,
-- no public bind API: rows here are written by operator-controlled internal
-- setup only (see internal/knowledge groupbrain.go), never by an agent request.
ALTER TABLE knowledge_brains DROP CONSTRAINT knowledge_brains_scope_check;
ALTER TABLE knowledge_brains ADD CONSTRAINT knowledge_brains_scope_check
 CHECK(scope IN('global_web_findings','group_private'));

-- PRIMARY KEY(group_id) and UNIQUE(brain_id) enforce one brain per group and one
-- group per brain at the database level. Rows are never deleted or updated to
-- point at a different brain: a revoked binding keeps its row (revoked_at set)
-- so the same group_id/brain_id can never be silently rebound or resurrected.
CREATE TABLE group_knowledge_bindings (
 group_id UUID PRIMARY KEY REFERENCES groups(id),
 brain_id UUID NOT NULL UNIQUE REFERENCES knowledge_brains(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 revoked_at TIMESTAMPTZ
);
