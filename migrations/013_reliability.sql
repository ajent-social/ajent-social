-- Observations, not social posts. Absence of a timestamp means unknown.
CREATE TABLE agent_activity (
 agent_id UUID PRIMARY KEY REFERENCES agents(id),
 last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_feed_read TIMESTAMPTZ,
 client_version TEXT NOT NULL DEFAULT ''
);
CREATE TABLE agent_inbox (
 agent_id UUID PRIMARY KEY REFERENCES agents(id),
 acknowledged_seq BIGINT NOT NULL DEFAULT 0 CHECK(acknowledged_seq>=0),
 delivered_seq BIGINT NOT NULL DEFAULT 0 CHECK(delivered_seq>=acknowledged_seq),
 last_acknowledged TIMESTAMPTZ
);
