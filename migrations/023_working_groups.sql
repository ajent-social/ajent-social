-- Purpose identities and canonical direct pairs survive membership changes.
ALTER TABLE groups ADD COLUMN purpose_key TEXT CHECK(length(purpose_key) BETWEEN 1 AND 128);
CREATE UNIQUE INDEX groups_purpose_key ON groups(org_id,purpose_key) WHERE purpose_key IS NOT NULL;
CREATE TABLE group_direct_keys (
 org_id UUID NOT NULL REFERENCES organizations(id),
 agent_low UUID NOT NULL REFERENCES agents(id),
 agent_high UUID NOT NULL REFERENCES agents(id),
 purpose_key TEXT NOT NULL CHECK(length(purpose_key) BETWEEN 1 AND 128),
 group_id UUID NOT NULL UNIQUE REFERENCES groups(id),
 PRIMARY KEY(org_id,agent_low,agent_high,purpose_key),
 CHECK(agent_low<agent_high)
);
CREATE TABLE group_subscriptions (
 agent_id UUID NOT NULL REFERENCES agents(id),
 group_id UUID NOT NULL REFERENCES groups(id),
 subscribed BOOLEAN NOT NULL,
 acknowledged_seq BIGINT NOT NULL CHECK(acknowledged_seq>=0),
 delivered_seq BIGINT NOT NULL CHECK(delivered_seq>=acknowledged_seq),
 PRIMARY KEY(agent_id,group_id)
);
