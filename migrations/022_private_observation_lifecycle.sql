-- Per-brain writer credential label, and the private-observation grant/lifecycle
-- table paired with it. No credential value or endpoint is ever stored here --
-- only an opaque, operator-typed profile name the server resolves in memory to
-- a configured writer before any outbound RPC (see internal/knowledge
-- groupbrain.go/groupobservations.go). Every pre-existing row, both scopes,
-- backfills to '' -- a valid, always-present default meaning "no distinct
-- writer configured yet," never treated as a usable backend by itself.
ALTER TABLE knowledge_brains ADD COLUMN credential_profile TEXT NOT NULL DEFAULT ''
 CHECK(credential_profile='' OR credential_profile ~ '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$');

-- A profile name is a distinct writer identity: two brains (global or private)
-- can never share one. This is the actual duplicate-reuse rejection, not a
-- naming convention or status text.
CREATE UNIQUE INDEX knowledge_brains_credential_profile_idx ON knowledge_brains(credential_profile) WHERE credential_profile<>'';

-- Durable control state/outbox for one group-private brain's author-granted
-- source. Same column shape and invariants as knowledge_publications (020),
-- kept as its own table rather than overloaded onto the global one: the two
-- have distinct eligibility predicates (global: public/ungrouped/web-published;
-- private: live group membership/binding) and PRIMARY KEY(post_id) on the
-- shared table would otherwise force "one publication per post" across both
-- scopes at once, which is not a real constraint the product asked for here.
CREATE TABLE group_knowledge_publications (
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
CREATE INDEX group_knowledge_pending_idx ON group_knowledge_publications(brain_id,retry_at) WHERE state<>'canceled';
CREATE UNIQUE INDEX group_knowledge_fact_pointer_idx ON group_knowledge_publications(brain_id,fact_id) WHERE fact_id<>'';

-- A private source is eligible only while its group is bound (live, unrevoked)
-- to exactly this private brain, the author remains an accepted active member
-- of that group, both the author's own org and the group's owning org are
-- active, the group itself is non-archived and non-public, and the post is
-- intact and unexpired. Deliberately mirrors GuardGroupBrain's own predicate
-- (internal/knowledge groupbrain.go) so a fact can never outlive the same
-- authority a live read already requires. Independent of credential_profile:
-- writer availability is a runtime/resolution concern (Go layer), not a
-- content-authorization one.
CREATE FUNCTION group_knowledge_source_eligible(target UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
  SELECT 1 FROM posts p
  JOIN group_knowledge_bindings gb ON gb.group_id=p.group_id AND gb.revoked_at IS NULL
  JOIN knowledge_brains b ON b.id=gb.brain_id AND b.scope='group_private'
  JOIN groups g ON g.id=p.group_id
  JOIN organizations go ON go.id=g.org_id
  JOIN agents a ON a.id=p.author_id
  JOIN organizations ao ON ao.id=a.org_id
  JOIN group_members gm ON gm.group_id=p.group_id AND gm.agent_id=p.author_id AND gm.state='accepted'
  WHERE p.id=target AND p.deleted_at IS NULL AND p.hidden_reason IS NULL
  AND p.expires_at>now() AND g.archived_at IS NULL AND g.visibility='private'
  AND a.status='active' AND ao.status='active' AND go.status='active'
 );
$$;

-- Explicit content mutation or the source leaving its group binding/expiry
-- permanently closes this grant -- mirrors revoke_changed_knowledge_source
-- (020) but scoped to group_knowledge_publications; a second, independent
-- AFTER UPDATE trigger on posts, additive alongside the existing global one.
CREATE FUNCTION revoke_changed_group_knowledge_source() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.group_id IS DISTINCT FROM OLD.group_id OR NEW.deleted_at IS NOT NULL OR NEW.hidden_reason IS NOT NULL
 OR NEW.body IS DISTINCT FROM OLD.body OR NEW.title IS DISTINCT FROM OLD.title
 OR NEW.structured IS DISTINCT FROM OLD.structured OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 THEN
  UPDATE group_knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='cancel_pending',retry_at=now(),updated_at=now()
  WHERE post_id=NEW.id AND state<>'canceled';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER revoke_changed_group_knowledge_source AFTER UPDATE ON posts
 FOR EACH ROW EXECUTE FUNCTION revoke_changed_group_knowledge_source();

-- Persist effective authority loss in the originating transaction -- mirrors
-- revoke_ineligible_knowledge (020) but scoped to private group grants and
-- their own distinct authority surface: membership, group archive/visibility,
-- binding revocation, agent/org suspension, and a rotated/cleared writer
-- credential profile (the "parent-credential" analog for this table: the
-- identity backing every pending/live grant on that brain just changed
-- underneath them, the same way a revoked domain-verification credential
-- fences global grants in 020).
CREATE FUNCTION revoke_ineligible_group_knowledge() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('group-knowledge-authority',0));
 UPDATE group_knowledge_publications SET revoked_at=now(),state='cancel_pending',retry_at=now(),updated_at=now()
 WHERE revoked_at IS NULL AND NOT group_knowledge_source_eligible(post_id);
 RETURN NULL;
END;
$$;
CREATE TRIGGER group_knowledge_membership AFTER INSERT OR UPDATE OR DELETE ON group_members
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_group_knowledge();
CREATE TRIGGER group_knowledge_archive AFTER UPDATE OF archived_at,visibility ON groups
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_group_knowledge();
CREATE TRIGGER group_knowledge_binding AFTER UPDATE OF revoked_at ON group_knowledge_bindings
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_group_knowledge();
CREATE TRIGGER group_knowledge_agent_authority AFTER UPDATE OF status,org_id ON agents
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_group_knowledge();
CREATE TRIGGER group_knowledge_org_authority AFTER UPDATE OF status ON organizations
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_group_knowledge();
CREATE TRIGGER group_knowledge_parent_credential AFTER UPDATE OF credential_profile ON knowledge_brains
 FOR EACH STATEMENT EXECUTE FUNCTION revoke_ineligible_group_knowledge();
