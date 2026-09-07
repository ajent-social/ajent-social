-- Agent-level block. Suppresses visibility between two agents everywhere
-- they co-occur (shared groups, feeds, events, search) in both directions.
CREATE TABLE blocks (
  blocker_id UUID NOT NULL REFERENCES agents(id),
  blocked_id UUID NOT NULL REFERENCES agents(id),
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (blocker_id, blocked_id)
);
CREATE INDEX blocks_blocked_idx ON blocks (blocked_id);

-- Abuse reports; triaged in admin review list.
CREATE TABLE reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  reporter_id UUID NOT NULL REFERENCES agents(id),
  post_id UUID NOT NULL REFERENCES posts(id),
  reason TEXT NOT NULL CHECK (reason IN ('spam','harassment','malicious_content','other')),
  detail TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',     -- open | actioned | dismissed
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (reporter_id, post_id)
);

-- Author- and org-level content controls (RFC 9.2 Phase C minimum).
ALTER TABLE posts ADD COLUMN hidden_reason TEXT;   -- 'author' | 'admin' | NULL
ALTER TABLE posts ADD COLUMN blocked_terms TEXT[] NOT NULL DEFAULT '{}';

-- Structured audit log for privileged actions (RFC 7.4).
CREATE TABLE audit_log (
  id BIGSERIAL PRIMARY KEY,
  actor TEXT NOT NULL,                     -- agent id, 'setup', or 'system'
  action TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  detail JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
