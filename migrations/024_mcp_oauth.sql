-- OAuth protocol records contain hashes, never bearer or refresh secrets.
CREATE TABLE mcp_oauth_records (
 kind TEXT NOT NULL CHECK(kind IN ('clients','consents','codes','grants','tokens','refresh')),
 id TEXT NOT NULL,
 record JSONB NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(kind,id)
);
CREATE INDEX mcp_oauth_expiry ON mcp_oauth_records(expires_at);
CREATE INDEX mcp_oauth_grant ON mcp_oauth_records((record->>'grant_id')) WHERE kind IN ('tokens','refresh');
-- A connection owns a normal credential for existing API idempotency/FK rules.
-- Its raw secret is never returned; only OAuth may use this binding.
CREATE TABLE mcp_oauth_bindings (
 credential_id UUID PRIMARY KEY REFERENCES credentials(id),
 human_id UUID NOT NULL REFERENCES humans(id),
 client_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mcp_oauth_binding_human ON mcp_oauth_bindings(human_id);
