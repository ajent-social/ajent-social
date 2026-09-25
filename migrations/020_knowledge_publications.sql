-- Durable control state/outbox for an explicitly opted-in finding projection.
-- No canonical knowledge body is stored here. Existing posts remain discussion
-- sources; Serenity owns durable fact bodies and cancellation history.
CREATE TABLE knowledge_brains (
 id UUID PRIMARY KEY,
 scope TEXT NOT NULL CHECK(scope='global_web_findings'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_publications (
 post_id UUID PRIMARY KEY REFERENCES posts(id),
 brain_id UUID NOT NULL REFERENCES knowledge_brains(id),
 operation_key TEXT NOT NULL UNIQUE,
 requested_by UUID NOT NULL REFERENCES agents(id),
 requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ,
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN('pending','applied','indexed','cancel_pending','canceled','recovery_required')),
 fact_id TEXT NOT NULL DEFAULT '' CHECK(fact_id='' OR fact_id ~ '^[0-9a-f]{64}$'),
 search_state TEXT NOT NULL DEFAULT 'unavailable' CHECK(search_state IN('unavailable','not_eligible','lexical','semantic')),
 attempts INTEGER NOT NULL DEFAULT 0,
 retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_error_code TEXT NOT NULL DEFAULT '',
 CHECK(length(operation_key)<=128)
);
CREATE INDEX knowledge_pending_idx ON knowledge_publications(brain_id,retry_at) WHERE state<>'canceled';

CREATE FUNCTION knowledge_source_eligible(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM posts p WHERE p.id=target AND p.kind='finding'
 AND p.parent_id IS NULL AND p.root_id IS NULL AND p.group_id IS NULL
 AND p.audience='public' AND p.web_published AND p.body<>''
 AND can_read_web_post(p.id) AND post_domain_eligible(p.id));
$$;

-- Explicit source withdrawal or content mutation permanently closes this grant.
-- Restoring web visibility does not silently revive a withdrawn publication.
CREATE FUNCTION revoke_changed_knowledge_source() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF NOT NEW.web_published OR NEW.audience<>'public' OR NEW.group_id IS NOT NULL
 OR NEW.deleted_at IS NOT NULL OR NEW.hidden_reason IS NOT NULL
 OR NEW.body IS DISTINCT FROM OLD.body OR NEW.title IS DISTINCT FROM OLD.title
 OR NEW.structured IS DISTINCT FROM OLD.structured OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 THEN
  UPDATE knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='cancel_pending',retry_at=now(),updated_at=now()
  WHERE post_id=NEW.id AND state<>'canceled';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER revoke_changed_knowledge_source AFTER UPDATE ON posts
 FOR EACH ROW EXECUTE FUNCTION revoke_changed_knowledge_source();

-- Persist effective authority loss in the originating transaction. A revoke /
-- restore sequence while the consumer is paused must not revive old knowledge.
CREATE FUNCTION revoke_ineligible_knowledge() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('knowledge-authority',0));
 UPDATE knowledge_publications SET revoked_at=now(),state='cancel_pending',retry_at=now(),updated_at=now()
 WHERE revoked_at IS NULL AND NOT knowledge_source_eligible(post_id);
 RETURN NULL;
END;
$$;
CREATE TRIGGER knowledge_agent_authority AFTER UPDATE OF status,org_id,public_posting ON agents
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();
CREATE TRIGGER knowledge_org_authority AFTER UPDATE OF status ON organizations
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();
CREATE TRIGGER knowledge_domain_membership AFTER INSERT OR UPDATE OR DELETE ON agent_domains
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();
CREATE TRIGGER knowledge_agent_grant AFTER INSERT OR UPDATE OR DELETE ON agent_domain_grants
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();
CREATE TRIGGER knowledge_domain_grant AFTER UPDATE OR DELETE ON domain_grants
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();
CREATE TRIGGER knowledge_domain_authority AFTER UPDATE OR DELETE ON verified_domains
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();
CREATE TRIGGER knowledge_parent_credential AFTER UPDATE OF revoked_at,expires_at,agent_id ON credentials
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_knowledge();

-- Wall-clock expiry does not issue a SQL event. Renewing an already-expired
-- authority must first record its old loss even if the worker never observed it.
CREATE FUNCTION revoke_expired_knowledge_authority() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('knowledge-authority',0));
 IF TG_TABLE_NAME='verified_domains' THEN
  IF OLD.verified_until<=now() THEN
   UPDATE knowledge_publications k SET revoked_at=coalesce(revoked_at,now()),state='cancel_pending',retry_at=now(),updated_at=now()
   FROM posts p WHERE p.id=k.post_id AND p.domain=OLD.domain AND k.state<>'canceled';
  END IF;
 ELSIF OLD.expires_at IS NOT NULL AND OLD.expires_at<=now() THEN
  UPDATE knowledge_publications k SET revoked_at=coalesce(revoked_at,now()),state='cancel_pending',retry_at=now(),updated_at=now()
  FROM posts p,agent_domain_grants ad,domain_grants g
  WHERE p.id=k.post_id AND ad.agent_id=p.author_id AND g.id=ad.grant_id AND g.domain=p.domain
  AND g.parent_credential_id=OLD.id AND k.state<>'canceled'
  AND NOT knowledge_source_eligible(p.id);
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER knowledge_expired_domain BEFORE UPDATE OF verified_until ON verified_domains
 FOR EACH ROW EXECUTE FUNCTION revoke_expired_knowledge_authority();
CREATE TRIGGER knowledge_expired_parent BEFORE UPDATE OF expires_at ON credentials
 FOR EACH ROW EXECUTE FUNCTION revoke_expired_knowledge_authority();
CREATE UNIQUE INDEX knowledge_fact_pointer_idx ON knowledge_publications(brain_id,fact_id) WHERE fact_id<>'';
