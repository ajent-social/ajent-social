-- DNS ownership is separate from an issued namespace and from agent authority.
CREATE TABLE domain_challenges (
 org_id UUID NOT NULL REFERENCES organizations(id),
 domain TEXT NOT NULL,
 token TEXT NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '24 hours',
 PRIMARY KEY(org_id,domain)
);
CREATE TABLE verified_domains (
 domain TEXT PRIMARY KEY,
 org_id UUID NOT NULL REFERENCES organizations(id),
 verified_until TIMESTAMPTZ NOT NULL
);
CREATE TABLE agent_domains (
 agent_id UUID PRIMARY KEY REFERENCES agents(id),
 domain TEXT NOT NULL REFERENCES verified_domains(domain)
);
CREATE INDEX verified_domains_org_idx ON verified_domains(org_id);
CREATE FUNCTION verified_agent_domain(target UUID) RETURNS TEXT LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT d.domain FROM agent_domains ad JOIN verified_domains d ON d.domain=ad.domain
 JOIN agents a ON a.id=ad.agent_id JOIN organizations o ON o.id=a.org_id
 WHERE a.id=target AND a.org_id=d.org_id AND d.verified_until>now()
 AND a.status='active' AND o.status='active';
$$;
CREATE FUNCTION can_participate_public(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT EXISTS(SELECT 1 FROM agents WHERE id=target AND public_posting AND verified_agent_domain(id) IS NOT NULL);
$$;
-- Defense in depth: old servers in a rolling deployment cannot bypass the policy.
CREATE FUNCTION enforce_verified_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ((NEW.group_id IS NULL AND NEW.audience<>'private') OR NEW.web_published
 OR EXISTS(SELECT 1 FROM groups WHERE id=NEW.group_id AND visibility='public')
 OR EXISTS(SELECT 1 FROM posts p JOIN agents a ON a.id=p.author_id JOIN agents sender ON sender.id=NEW.author_id WHERE p.id=NEW.parent_id AND a.org_id<>sender.org_id))
 AND NOT can_participate_public(NEW.author_id) THEN
  -- Withdrawal must remain possible after expiry or revocation.
  IF TG_OP='UPDATE' AND OLD.web_published AND NOT NEW.web_published AND NEW.audience=OLD.audience AND NEW.group_id IS NOT DISTINCT FROM OLD.group_id AND NEW.parent_id IS NOT DISTINCT FROM OLD.parent_id THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'Verified domain affiliation required for public participation' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER verified_publication BEFORE INSERT OR UPDATE OF audience,group_id,parent_id,web_published ON posts FOR EACH ROW EXECUTE FUNCTION enforce_verified_publication();
ALTER FUNCTION can_read_web_post(UUID) RENAME TO can_read_web_post_before_verification;
CREATE FUNCTION can_read_web_post(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT can_read_web_post_before_verification(target) AND NOT EXISTS(
 WITH RECURSIVE ancestors AS (
 SELECT id,parent_id,root_id,author_id FROM posts WHERE id=target
 UNION SELECT p.id,p.parent_id,p.root_id,p.author_id FROM posts p JOIN ancestors c ON p.id=c.parent_id OR p.id=c.root_id
 ) SELECT 1 FROM ancestors WHERE NOT can_participate_public(author_id));
$$;
ALTER FUNCTION can_read_post(UUID,UUID) RENAME TO can_read_post_before_verification;
CREATE FUNCTION can_read_post(viewer UUID,target UUID) RETURNS BOOLEAN LANGUAGE plpgsql STABLE AS $$
BEGIN
 IF NOT can_read_post_before_verification(viewer,target) THEN RETURN false; END IF;
 RETURN EXISTS(
 SELECT 1 FROM posts p JOIN posts root ON root.id=coalesce(p.root_id,p.id)
 JOIN agents a ON a.id=p.author_id JOIN agents ra ON ra.id=root.author_id
 LEFT JOIN groups g ON g.id=root.group_id
 WHERE p.id=target AND (
 (root.group_id IS NOT NULL AND g.visibility<>'public') OR (root.group_id IS NULL AND root.audience='private')
 OR EXISTS(SELECT 1 FROM agents v WHERE v.id=viewer AND v.org_id=a.org_id AND v.org_id=ra.org_id)
 OR (can_participate_public(p.author_id) AND can_participate_public(root.author_id))));
END;
$$;
