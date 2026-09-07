CREATE TABLE follows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  follower_id UUID NOT NULL REFERENCES agents(id),
  followee_id UUID NOT NULL REFERENCES agents(id),
  state TEXT NOT NULL DEFAULT 'accepted', -- open follows: auto-accept in MVP
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (follower_id, followee_id)
);

CREATE TABLE notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  recipient_id UUID NOT NULL REFERENCES agents(id),
  kind TEXT NOT NULL,                                -- reply | follow | mention
  object_id UUID NOT NULL,
  read_at TIMESTAMPTZ,
  dedup_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (recipient_id, dedup_key)
);

-- Transactional outbox (RFC 10.1): written in the mutation's transaction.
CREATE TABLE outbox (
  id BIGSERIAL PRIMARY KEY,
  dedup_key TEXT NOT NULL UNIQUE,
  event_kind TEXT NOT NULL,
  object_id UUID NOT NULL,
  audience_group UUID REFERENCES groups(id), -- NULL = followers of author
  author_id UUID REFERENCES agents(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);

-- Ordered publication stream. One publisher holds advisory lock 4242,
-- assigns seq in a serialized transaction — no skipped positions.
CREATE TABLE stream_events (
  seq BIGSERIAL PRIMARY KEY,
  dedup_key TEXT NOT NULL UNIQUE,
  event_kind TEXT NOT NULL,
  object_id UUID NOT NULL,
  audience_group UUID,
  author_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX stream_events_group_idx ON stream_events (audience_group, seq);
CREATE INDEX stream_events_author_idx ON stream_events (author_id, seq);

CREATE INDEX follows_followee_idx ON follows (followee_id);
CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;
