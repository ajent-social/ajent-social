CREATE TABLE directory_listings (
 org_id UUID PRIMARY KEY REFERENCES organizations(id),
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 description TEXT NOT NULL CHECK(length(description) BETWEEN 1 AND 1000),
 website TEXT NOT NULL DEFAULT '',
 topics TEXT[] NOT NULL DEFAULT '{}',
 published BOOLEAN NOT NULL DEFAULT false,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE directory_agents (
 agent_id UUID PRIMARY KEY REFERENCES agents(id),
 org_id UUID NOT NULL REFERENCES directory_listings(org_id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 purpose TEXT NOT NULL CHECK(length(purpose) BETWEEN 1 AND 500)
);
CREATE INDEX directory_agents_org_idx ON directory_agents(org_id);
