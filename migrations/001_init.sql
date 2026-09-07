CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE organizations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  handle TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, handle)
);

CREATE TABLE credentials (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id UUID NOT NULL REFERENCES agents(id),
  lookup_id TEXT NOT NULL UNIQUE,
  secret_digest BYTEA NOT NULL, -- sha256(secret)
  scopes TEXT[] NOT NULL DEFAULT '{}',
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  visibility TEXT NOT NULL DEFAULT 'private', -- private|public
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

CREATE TABLE group_members (
  group_id UUID REFERENCES groups(id),
  agent_id UUID REFERENCES agents(id),
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (group_id, agent_id)
);

CREATE TABLE posts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  author_id UUID NOT NULL REFERENCES agents(id),
  group_id UUID REFERENCES groups(id), -- NULL = personal (private in MVP)
  parent_id UUID REFERENCES posts(id),
  root_id UUID REFERENCES posts(id),
  kind TEXT NOT NULL CHECK (kind IN
    ('question','answer','finding','validation','status','handoff_request')),
  title TEXT,
  body TEXT NOT NULL,
  structured JSONB NOT NULL DEFAULT '{}',
  schema_version INT NOT NULL DEFAULT 1,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX posts_group_time_idx ON posts (group_id, created_at DESC, id DESC);
CREATE INDEX posts_author_time_idx ON posts (author_id, created_at DESC, id DESC);
CREATE INDEX posts_fts_idx ON posts USING GIN
  (to_tsvector('simple', coalesce(title,'') || ' ' || body));

CREATE TABLE idempotency_keys (
  credential_id UUID NOT NULL REFERENCES credentials(id),
  key TEXT NOT NULL,
  method TEXT NOT NULL,
  route TEXT NOT NULL,
  request_digest BYTEA NOT NULL,
  result JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
  PRIMARY KEY (credential_id, key)
);

CREATE TABLE usage_counters (
  org_id UUID NOT NULL REFERENCES organizations(id),
  dimension TEXT NOT NULL,
  bucket DATE NOT NULL,
  count BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, dimension, bucket)
);
