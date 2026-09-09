ALTER TABLE groups ADD COLUMN rules TEXT NOT NULL DEFAULT '' CHECK(octet_length(rules)<=8000);
ALTER TABLE groups ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE groups ADD COLUMN archived_at TIMESTAMPTZ;
CREATE INDEX groups_listing_idx ON groups(created_at DESC,id DESC);

CREATE TABLE group_pins (
 group_id UUID NOT NULL REFERENCES groups(id),
 post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
 position SMALLINT NOT NULL CHECK(position BETWEEN 1 AND 3),
 PRIMARY KEY(group_id,post_id),
 UNIQUE(group_id,position)
);

-- Serialize membership changes and all post writers with lifecycle changes.
-- This applies to API, browser, jobs and future writers, not only one handler.
CREATE FUNCTION guard_group_membership() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM groups WHERE id=coalesce(NEW.group_id,OLD.group_id) FOR UPDATE;
 RETURN coalesce(NEW,OLD);
END;
$$;
CREATE TRIGGER guard_group_membership BEFORE INSERT OR UPDATE OR DELETE ON group_members
 FOR EACH ROW EXECUTE FUNCTION guard_group_membership();

CREATE FUNCTION preserve_group_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.role='owner' AND OLD.state='accepted' AND EXISTS(SELECT 1 FROM groups WHERE id=OLD.group_id)
 AND NOT EXISTS(SELECT 1 FROM group_members WHERE group_id=OLD.group_id AND role='owner' AND state='accepted') THEN
  RAISE EXCEPTION 'Transfer group ownership before leaving' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER preserve_group_owner AFTER UPDATE OR DELETE ON group_members
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION preserve_group_owner();

CREATE FUNCTION guard_group_post() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE archived TIMESTAMPTZ;
BEGIN
 IF NEW.group_id IS NULL THEN RETURN NEW; END IF;
 SELECT archived_at INTO archived FROM groups WHERE id=NEW.group_id FOR SHARE;
 -- Exact content erasure remains possible for retention cleanup while archived.
 IF TG_OP='UPDATE' AND NEW.body='' AND coalesce(NEW.title,'')='' AND NEW.structured='{}'::jsonb
 AND NEW.deleted_at IS NOT NULL AND NEW.group_id IS NOT DISTINCT FROM OLD.group_id
 AND NEW.author_id=OLD.author_id AND NEW.parent_id IS NOT DISTINCT FROM OLD.parent_id
 AND NEW.root_id IS NOT DISTINCT FROM OLD.root_id THEN RETURN NEW; END IF;
 -- Retraction/expiry/moderation remain possible while archived.
 IF archived IS NOT NULL AND (TG_OP='INSERT' OR NEW.body IS DISTINCT FROM OLD.body
 OR NEW.title IS DISTINCT FROM OLD.title OR NEW.structured IS DISTINCT FROM OLD.structured
 OR NEW.group_id IS DISTINCT FROM OLD.group_id) THEN
  RAISE EXCEPTION 'Group is archived' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER guard_group_post BEFORE INSERT OR UPDATE ON posts FOR EACH ROW EXECUTE FUNCTION guard_group_post();

CREATE FUNCTION guard_group_pin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM groups WHERE id=NEW.group_id AND archived_at IS NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Group is archived' USING ERRCODE='23514'; END IF;
 PERFORM 1 FROM posts WHERE id=NEW.post_id AND group_id=NEW.group_id AND parent_id IS NULL
 AND deleted_at IS NULL AND hidden_reason IS NULL AND expires_at>now() FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Pin requires a live same-group root' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER guard_group_pin BEFORE INSERT OR UPDATE ON group_pins FOR EACH ROW EXECUTE FUNCTION guard_group_pin();
