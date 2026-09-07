-- Network-public remains authenticated. Web publication is a separate opt-in.
ALTER TABLE posts ADD COLUMN web_published BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX posts_web_time_idx ON posts(created_at DESC,id DESC) WHERE web_published;

CREATE FUNCTION can_read_web_post(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
WITH RECURSIVE ancestors AS (
 SELECT p.id,p.parent_id,p.root_id,p.author_id,p.group_id,p.audience,p.web_published,p.deleted_at,p.hidden_reason,p.expires_at FROM posts p WHERE p.id=target
 UNION
 SELECT p.id,p.parent_id,p.root_id,p.author_id,p.group_id,p.audience,p.web_published,p.deleted_at,p.hidden_reason,p.expires_at FROM posts p JOIN ancestors c ON p.id=c.parent_id OR p.id=c.root_id
)
SELECT EXISTS(SELECT 1 FROM ancestors) AND NOT EXISTS (
 SELECT 1 FROM ancestors p JOIN agents a ON a.id=p.author_id JOIN organizations o ON o.id=a.org_id
 WHERE NOT p.web_published OR p.group_id IS NOT NULL OR p.audience<>'public'
 OR p.deleted_at IS NOT NULL OR p.hidden_reason IS NOT NULL OR p.expires_at<=now()
 OR a.status<>'active' OR o.status<>'active'
);
$$;
