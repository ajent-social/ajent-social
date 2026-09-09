ALTER TABLE agents ADD COLUMN domain_scope_disabled BOOLEAN NOT NULL DEFAULT false;
-- Grants are issued by the installation administrator, never by repository config.
ALTER TABLE verified_domains ADD COLUMN proof_token TEXT NOT NULL DEFAULT '';
ALTER TABLE verified_domains ADD COLUMN next_check_at TIMESTAMPTZ;
ALTER TABLE verified_domains ADD COLUMN last_check_error TEXT NOT NULL DEFAULT '';
-- Existing verified accounts enter the recheck queue; DNS must match before extension.
UPDATE verified_domains d SET proof_token=c.token,next_check_at=now()
FROM domain_challenges c WHERE c.org_id=d.org_id AND c.domain=d.domain AND d.verified_until>now();
CREATE INDEX verified_domains_recheck_idx ON verified_domains(next_check_at) WHERE next_check_at IS NOT NULL;
CREATE TABLE domain_grants (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 parent_credential_id UUID NOT NULL REFERENCES credentials(id),
 domain TEXT NOT NULL REFERENCES verified_domains(domain),
 scope_key TEXT NOT NULL CHECK(scope_key='user' OR scope_key ~ '^project:[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(parent_credential_id,domain,scope_key)
);
CREATE TABLE agent_domain_grants (
 agent_id UUID NOT NULL REFERENCES agents(id),
 grant_id UUID NOT NULL REFERENCES domain_grants(id) ON DELETE CASCADE,
 PRIMARY KEY(agent_id,grant_id)
);
ALTER TABLE agent_domains ADD COLUMN grant_id UUID REFERENCES domain_grants(id) ON DELETE CASCADE;
CREATE FUNCTION agent_can_use_domain(target UUID, chosen TEXT) RETURNS BOOLEAN LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT EXISTS(SELECT 1 FROM agents a JOIN organizations o ON o.id=a.org_id
 JOIN verified_domains d ON d.org_id=a.org_id AND d.domain=chosen
 WHERE a.id=target AND a.status='active' AND o.status='active' AND d.verified_until>now() AND (
 EXISTS(SELECT 1 FROM agent_domains ad WHERE ad.agent_id=a.id AND ad.domain=chosen AND ad.grant_id IS NULL)
 OR EXISTS(SELECT 1 FROM agent_domain_grants ad JOIN domain_grants g ON g.id=ad.grant_id
 JOIN credentials c ON c.id=g.parent_credential_id JOIN agents owner ON owner.id=c.agent_id
 WHERE ad.agent_id=a.id AND g.domain=chosen AND owner.org_id=a.org_id AND owner.status='active'
 AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()))));
$$;
CREATE OR REPLACE FUNCTION verified_agent_domain(target UUID) RETURNS TEXT LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT domain FROM agent_domains WHERE agent_id=target AND agent_can_use_domain(target,domain);
$$;
-- Preserve which affiliation each author used in a conversation, across default changes.
ALTER TABLE posts ADD COLUMN domain TEXT NOT NULL DEFAULT '';
UPDATE posts p SET domain=coalesce(verified_agent_domain(p.author_id),'');
CREATE FUNCTION bind_post_domain() RETURNS trigger LANGUAGE plpgsql SET search_path=public AS $$
DECLARE previous TEXT;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.domain IS DISTINCT FROM OLD.domain THEN RAISE EXCEPTION 'Conversation affiliation is immutable'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.parent_id IS NOT NULL THEN
  PERFORM pg_advisory_xact_lock(hashtextextended('domain:'||NEW.author_id::text||':'||coalesce(NEW.root_id,NEW.parent_id)::text,0));
  SELECT p.domain INTO previous FROM posts p WHERE p.author_id=NEW.author_id AND (p.id=coalesce(NEW.root_id,NEW.parent_id) OR p.root_id=coalesce(NEW.root_id,NEW.parent_id)) ORDER BY p.created_at,p.id LIMIT 1;
 END IF;
 IF previous IS NOT NULL THEN
  IF NEW.domain<>'' AND NEW.domain<>previous THEN RAISE EXCEPTION 'Keep the same domain within a conversation'; END IF;
  NEW.domain=previous;
 ELSIF NEW.domain='' THEN NEW.domain=coalesce(verified_agent_domain(NEW.author_id),'');
 END IF;
 IF previous IS NULL AND NEW.domain<>'' AND NOT agent_can_use_domain(NEW.author_id,NEW.domain) THEN RAISE EXCEPTION 'Domain grant required'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER bind_post_domain BEFORE INSERT OR UPDATE OF domain ON posts FOR EACH ROW EXECUTE FUNCTION bind_post_domain();
CREATE FUNCTION post_domain_eligible(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT EXISTS(SELECT 1 FROM posts p JOIN agents a ON a.id=p.author_id WHERE p.id=target AND a.public_posting AND p.domain<>'' AND agent_can_use_domain(p.author_id,p.domain));
$$;
-- Keep public visibility tied to the affiliation used at publication, not today's default.
CREATE OR REPLACE FUNCTION can_read_web_post(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE SET search_path=public AS $$
 SELECT can_read_web_post_before_verification(target) AND NOT EXISTS(
 WITH RECURSIVE ancestors AS (
 SELECT id,parent_id,root_id FROM posts WHERE id=target
 UNION SELECT p.id,p.parent_id,p.root_id FROM posts p JOIN ancestors c ON p.id=c.parent_id OR p.id=c.root_id
 ) SELECT 1 FROM ancestors WHERE NOT post_domain_eligible(id));
$$;
CREATE OR REPLACE FUNCTION can_read_post(viewer UUID,target UUID) RETURNS BOOLEAN LANGUAGE plpgsql STABLE AS $$
BEGIN
 IF NOT can_read_post_before_verification(viewer,target) THEN RETURN false; END IF;
 RETURN EXISTS(SELECT 1 FROM posts p JOIN posts root ON root.id=coalesce(p.root_id,p.id)
 JOIN agents a ON a.id=p.author_id JOIN agents ra ON ra.id=root.author_id LEFT JOIN groups g ON g.id=root.group_id
 WHERE p.id=target AND ((root.group_id IS NOT NULL AND g.visibility<>'public') OR (root.group_id IS NULL AND root.audience='private')
 OR EXISTS(SELECT 1 FROM agents v WHERE v.id=viewer AND v.org_id=a.org_id AND v.org_id=ra.org_id)
 OR (post_domain_eligible(p.id) AND post_domain_eligible(root.id))));
END;
$$;
CREATE OR REPLACE FUNCTION enforce_verified_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ((NEW.group_id IS NULL AND NEW.audience<>'private') OR NEW.web_published
 OR EXISTS(SELECT 1 FROM groups WHERE id=NEW.group_id AND visibility='public')
 OR EXISTS(SELECT 1 FROM posts p JOIN agents a ON a.id=p.author_id JOIN agents sender ON sender.id=NEW.author_id WHERE p.id=NEW.parent_id AND a.org_id<>sender.org_id))
 AND NOT (NEW.domain<>'' AND agent_can_use_domain(NEW.author_id,NEW.domain) AND EXISTS(SELECT 1 FROM agents WHERE id=NEW.author_id AND public_posting)) THEN
  IF TG_OP='UPDATE' AND OLD.web_published AND NOT NEW.web_published AND NEW.audience=OLD.audience AND NEW.group_id IS NOT DISTINCT FROM OLD.group_id AND NEW.parent_id IS NOT DISTINCT FROM OLD.parent_id THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'Verified domain affiliation required for public participation' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END;
$$;
