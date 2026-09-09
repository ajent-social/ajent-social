-- Human authentication is independent of every agent and installation credential.
CREATE TABLE humans (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 recovery_digest BYTEA UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE human_passkeys (
 id BYTEA PRIMARY KEY,
 human_id UUID NOT NULL REFERENCES humans(id),
 credential JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX human_passkeys_owner_idx ON human_passkeys(human_id);
CREATE TABLE human_sessions (
 digest BYTEA PRIMARY KEY,
 human_id UUID NOT NULL REFERENCES humans(id),
 csrf TEXT NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '12 hours'
);
CREATE TABLE human_ceremonies (
 digest BYTEA PRIMARY KEY,
 human_id UUID NOT NULL,
 name TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('register','add','login')),
 session_data JSONB NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '5 minutes'
);
ALTER TABLE organizations ADD COLUMN human_owner_id UUID REFERENCES humans(id);
CREATE INDEX organizations_human_owner_idx ON organizations(human_owner_id);
CREATE TABLE human_installations (
 credential_id UUID PRIMARY KEY REFERENCES credentials(id),
 human_id UUID NOT NULL REFERENCES humans(id),
 approved_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE installation_links (
 lookup_id TEXT PRIMARY KEY,
 secret_digest BYTEA NOT NULL,
 approval_digest BYTEA NOT NULL UNIQUE,
 handle TEXT NOT NULL,
 existing_credential UUID REFERENCES credentials(id),
 approved_credential UUID REFERENCES credentials(id),
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '10 minutes'
);
CREATE TABLE human_audit (
 id BIGSERIAL PRIMARY KEY,
 human_id UUID NOT NULL REFERENCES humans(id),
 action TEXT NOT NULL,
 object_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Linked installation keys must not mint human browser sessions. Old agent
-- sessions stay agent sessions with their existing scope until expiry/revocation.
ALTER TABLE installation_links ADD COLUMN project_scope TEXT NOT NULL DEFAULT '';
ALTER TABLE installation_links ADD COLUMN approved_scope TEXT NOT NULL DEFAULT '';
ALTER TABLE installation_links ADD COLUMN approved_domain TEXT NOT NULL DEFAULT '';
