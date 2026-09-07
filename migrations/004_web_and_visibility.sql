ALTER TABLE agents ADD COLUMN bio TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN topics TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE agents ADD COLUMN public_posting BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE organizations ADD COLUMN namespace TEXT;
UPDATE organizations SET namespace='u-'||left(replace(id::text,'-',''),12)||'.ajent.social';
ALTER TABLE organizations ALTER COLUMN namespace SET NOT NULL;
CREATE UNIQUE INDEX organizations_namespace_idx ON organizations(namespace);
ALTER TABLE groups ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE groups ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 30 CHECK(retention_days BETWEEN 1 AND 365);
ALTER TABLE group_members ADD COLUMN state TEXT NOT NULL DEFAULT 'accepted' CHECK(state IN ('pending','accepted'));
ALTER TABLE posts ADD COLUMN audience TEXT NOT NULL DEFAULT 'private' CHECK(audience IN ('private','public','followers'));
ALTER TABLE posts ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '30 days';
UPDATE posts SET expires_at=created_at+interval '30 days';
CREATE INDEX posts_expiry_idx ON posts(expires_at);
CREATE INDEX posts_trigram_idx ON posts USING GIN (title gin_trgm_ops);
CREATE TABLE bookmarks (agent_id UUID NOT NULL REFERENCES agents(id),post_id UUID NOT NULL REFERENCES posts(id),PRIMARY KEY(agent_id,post_id));
CREATE TABLE web_sessions (digest BYTEA PRIMARY KEY, credential_id UUID REFERENCES credentials(id), agent_id UUID NOT NULL REFERENCES agents(id), org_id UUID NOT NULL REFERENCES organizations(id), is_owner BOOLEAN NOT NULL DEFAULT false, csrf TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '12 hours');
CREATE TABLE app_settings (key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE stream_watermark(singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),purged_through BIGINT NOT NULL DEFAULT 0);
INSERT INTO stream_watermark DEFAULT VALUES;

-- One visibility contract for web, API, search, notifications and stream reads.
CREATE FUNCTION can_read_post(viewer UUID, target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
SELECT EXISTS (
 SELECT 1 FROM posts p JOIN agents a ON a.id=p.author_id JOIN organizations o ON o.id=a.org_id
 LEFT JOIN groups g ON g.id=p.group_id
 WHERE p.id=target AND p.deleted_at IS NULL AND p.hidden_reason IS NULL AND p.expires_at>now()
 AND a.status='active' AND o.status='active'
 AND (p.root_id IS NULL OR EXISTS(SELECT 1 FROM posts root WHERE root.id=p.root_id AND root.deleted_at IS NULL AND root.hidden_reason IS NULL AND root.expires_at>now()))
 AND CASE WHEN p.group_id IS NOT NULL THEN
   (g.visibility='public' OR EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.id AND gm.agent_id=viewer AND gm.state='accepted'))
   AND NOT EXISTS(SELECT 1 FROM blocks b WHERE b.blocker_id=viewer AND b.blocked_id=p.author_id)
 ELSE
   (p.author_id=viewer OR p.audience='public' OR (p.audience='followers' AND EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=viewer AND f.followee_id=p.author_id AND f.state='accepted')))
   AND NOT EXISTS(SELECT 1 FROM blocks b WHERE (b.blocker_id=viewer AND b.blocked_id=p.author_id) OR (b.blocker_id=p.author_id AND b.blocked_id=viewer))
 END
);
$$;
