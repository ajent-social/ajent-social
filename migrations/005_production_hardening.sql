-- Bind privileged sessions to the owner key that issued them. Existing owner
-- sessions deliberately expire on upgrade; agent sessions are unaffected.
ALTER TABLE web_sessions ADD COLUMN owner_key_digest BYTEA;
DELETE FROM web_sessions WHERE is_owner;

CREATE INDEX posts_root_time_idx ON posts(root_id,created_at DESC,id DESC);
CREATE INDEX posts_feed_time_idx ON posts(created_at DESC,id DESC) WHERE parent_id IS NULL;
CREATE INDEX notifications_recipient_time_idx ON notifications(recipient_id,created_at DESC);
CREATE INDEX notifications_object_idx ON notifications(object_id);
CREATE INDEX bookmarks_post_idx ON bookmarks(post_id);
CREATE INDEX web_sessions_expiry_idx ON web_sessions(expires_at);
CREATE INDEX idempotency_expiry_idx ON idempotency_keys(expires_at);
CREATE INDEX stream_events_created_idx ON stream_events(created_at,seq);
CREATE INDEX outbox_published_created_idx ON outbox(created_at) WHERE published_at IS NOT NULL;
CREATE INDEX credentials_agent_idx ON credentials(agent_id);
CREATE INDEX group_members_agent_idx ON group_members(agent_id);

-- A reply belongs to its root's audience. Check both authors' current state and
-- blocks without recursion, so malformed imported ancestry cannot recurse.
CREATE OR REPLACE FUNCTION can_read_post(viewer UUID, target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
SELECT EXISTS (
 SELECT 1 FROM posts p
 JOIN posts root ON root.id=coalesce(p.root_id,p.id)
 JOIN agents a ON a.id=p.author_id JOIN organizations o ON o.id=a.org_id
 JOIN agents ra ON ra.id=root.author_id JOIN organizations ro ON ro.id=ra.org_id
 LEFT JOIN groups g ON g.id=root.group_id
 WHERE p.id=target AND p.deleted_at IS NULL AND p.hidden_reason IS NULL AND p.expires_at>now()
 AND root.root_id IS NULL AND root.deleted_at IS NULL AND root.hidden_reason IS NULL AND root.expires_at>now()
 AND a.status='active' AND o.status='active' AND ra.status='active' AND ro.status='active'
 AND p.group_id IS NOT DISTINCT FROM root.group_id AND p.audience=root.audience
 AND CASE WHEN root.group_id IS NOT NULL THEN
   (g.visibility='public' OR EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.id AND gm.agent_id=viewer AND gm.state='accepted'))
   AND NOT EXISTS(SELECT 1 FROM blocks b WHERE b.blocker_id=viewer AND b.blocked_id IN(p.author_id,root.author_id))
 ELSE
   (root.author_id=viewer OR root.audience='public' OR (root.audience='followers' AND EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=viewer AND f.followee_id=root.author_id AND f.state='accepted')))
   AND NOT EXISTS(SELECT 1 FROM blocks b WHERE (b.blocker_id=viewer AND b.blocked_id IN(p.author_id,root.author_id)) OR (b.blocker_id IN(p.author_id,root.author_id) AND b.blocked_id=viewer))
 END
);
$$;
