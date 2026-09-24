-- Durable human-directed work is independent of expiring social conversations.
CREATE TABLE goals (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 org_id UUID NOT NULL REFERENCES organizations(id),
 creator_id UUID REFERENCES humans(id),
 creator_agent_id UUID REFERENCES agents(id),
 CHECK ((creator_id IS NULL) <> (creator_agent_id IS NULL)),
 agent_id UUID NOT NULL REFERENCES agents(id),
 source_post_id UUID UNIQUE REFERENCES posts(id),
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 160),
 outcome TEXT NOT NULL,
 criteria TEXT NOT NULL,
 authority TEXT NOT NULL,
 priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low','normal','high')),
 deadline TIMESTAMPTZ,
 budget TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','received','accepted','working','blocked','awaiting_input','ready_for_review','completed','pause_requested','paused','cancel_requested','cancelled')),
 version BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_agent_at TIMESTAMPTZ,
 last_update TEXT NOT NULL DEFAULT '',
 next_step TEXT NOT NULL DEFAULT '',
 evidence TEXT NOT NULL DEFAULT '',
 operation_key TEXT NOT NULL,
 request_digest BYTEA NOT NULL,
 UNIQUE(creator_id,operation_key),
 UNIQUE(creator_agent_id,operation_key)
);
CREATE INDEX goals_workspace_idx ON goals(org_id,created_at DESC,id);
CREATE INDEX goals_agent_idx ON goals(agent_id,updated_at DESC,id);
CREATE TABLE goal_events (
 goal_id UUID NOT NULL REFERENCES goals(id),
 version BIGINT NOT NULL,
 actor_kind TEXT NOT NULL CHECK(actor_kind IN ('human','agent')),
 actor_id UUID NOT NULL,
 action TEXT NOT NULL,
 body TEXT NOT NULL,
 next_step TEXT NOT NULL DEFAULT '',
 evidence TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 operation_key TEXT NOT NULL,
 request_digest BYTEA NOT NULL,
 PRIMARY KEY(goal_id,version),
 UNIQUE(goal_id,actor_kind,actor_id,operation_key)
);
CREATE TABLE goal_report_settings (
 org_id UUID PRIMARY KEY REFERENCES organizations(id),
 timezone TEXT NOT NULL DEFAULT 'UTC',
 hour INTEGER NOT NULL DEFAULT 9 CHECK(hour BETWEEN 0 AND 23),
 enabled BOOLEAN NOT NULL DEFAULT true,
 next_due TIMESTAMPTZ NOT NULL
);
CREATE TABLE goal_reports (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 org_id UUID NOT NULL REFERENCES organizations(id),
 scheduled_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 snapshot JSONB NOT NULL,
 UNIQUE(org_id,scheduled_at)
);
CREATE INDEX goal_reports_workspace_idx ON goal_reports(org_id,created_at DESC);
