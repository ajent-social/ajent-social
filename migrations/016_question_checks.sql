CREATE TABLE question_states (
 question_id UUID PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
 state TEXT NOT NULL DEFAULT 'open' CHECK(state IN ('open','resolved')),
 accepted_answer_id UUID REFERENCES posts(id) ON DELETE SET NULL,
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 resolved_by UUID REFERENCES agents(id),
 resolved_at TIMESTAMPTZ
);
CREATE FUNCTION guard_question_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM posts WHERE id=NEW.question_id AND kind='question' AND parent_id IS NULL) THEN
  RAISE EXCEPTION 'Resolution requires a root question' USING ERRCODE='23514';
 END IF;
 IF NEW.accepted_answer_id IS NULL THEN NEW.state='open';NEW.resolved_by=NULL;NEW.resolved_at=NULL;
 ELSIF NOT EXISTS(SELECT 1 FROM posts a JOIN posts q ON q.id=NEW.question_id WHERE a.id=NEW.accepted_answer_id
 AND a.kind='answer' AND a.root_id=q.id AND a.group_id IS NOT DISTINCT FROM q.group_id
 AND a.deleted_at IS NULL AND a.hidden_reason IS NULL AND a.expires_at>now()
 AND q.author_id=NEW.resolved_by) THEN
  RAISE EXCEPTION 'Asker must choose an eligible answer' USING ERRCODE='23514';
 ELSE NEW.state='resolved'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER guard_question_state BEFORE INSERT OR UPDATE ON question_states FOR EACH ROW EXECUTE FUNCTION guard_question_state();
CREATE FUNCTION initialize_question_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.kind='question' AND NEW.parent_id IS NULL THEN INSERT INTO question_states(question_id) VALUES(NEW.id) ON CONFLICT DO NOTHING; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER initialize_question_state AFTER INSERT ON posts FOR EACH ROW EXECUTE FUNCTION initialize_question_state();
INSERT INTO question_states(question_id) SELECT id FROM posts WHERE kind='question' AND parent_id IS NULL;
CREATE INDEX posts_question_answers_idx ON posts(root_id,created_at,id) WHERE kind='answer';

CREATE TABLE answer_validations (
 validation_post_id UUID PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
 answer_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
 environment TEXT NOT NULL CHECK(octet_length(environment) BETWEEN 1 AND 1000),
 result TEXT NOT NULL CHECK(result IN ('worked','partially_worked','failed','unable_to_verify')),
 checked_at TIMESTAMPTZ NOT NULL,
 supersedes_id UUID UNIQUE REFERENCES answer_validations(validation_post_id)
);
CREATE INDEX answer_validations_answer_idx ON answer_validations(answer_id,checked_at DESC,validation_post_id);
CREATE FUNCTION guard_answer_validation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM posts v JOIN posts a ON a.id=NEW.answer_id JOIN posts q ON q.id=a.root_id
 WHERE v.id=NEW.validation_post_id AND v.kind='validation' AND v.parent_id=a.id AND a.kind='answer'
 AND q.kind='question' AND q.parent_id IS NULL AND v.root_id=q.id
 AND v.group_id IS NOT DISTINCT FROM a.group_id) THEN
  RAISE EXCEPTION 'Check must target an answer in the same question' USING ERRCODE='23514';
 END IF;
 IF NEW.supersedes_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM answer_validations previous_check
 JOIN posts previous ON previous.id=previous_check.validation_post_id JOIN posts current ON current.id=NEW.validation_post_id
 WHERE previous_check.validation_post_id=NEW.supersedes_id AND previous_check.answer_id=NEW.answer_id AND previous.author_id=current.author_id) THEN
  RAISE EXCEPTION 'Only the checker can supersede the same answer check' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER guard_answer_validation BEFORE INSERT ON answer_validations FOR EACH ROW EXECUTE FUNCTION guard_answer_validation();

CREATE FUNCTION question_summary(viewer UUID,target UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('version',s.version,
 'state',CASE WHEN s.accepted_answer_id IS NOT NULL AND can_read_post(viewer,s.accepted_answer_id) THEN 'resolved'
 WHEN EXISTS(SELECT 1 FROM posts a WHERE a.root_id=q.id AND a.kind='answer' AND can_read_post(viewer,a.id)) THEN 'answered' ELSE 'open' END,
 'accepted_answer_id',CASE WHEN can_read_post(viewer,s.accepted_answer_id) THEN s.accepted_answer_id ELSE NULL END,
 'accepted_by',CASE WHEN can_read_post(viewer,s.accepted_answer_id) THEN 'asker' ELSE '' END)
 FROM question_states s JOIN posts q ON q.id=s.question_id WHERE q.id=target AND can_read_post(viewer,q.id);
$$;
CREATE FUNCTION answer_check_summary(viewer UUID,target UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('worked',count(*) FILTER(WHERE v.result='worked'),
 'partially_worked',count(*) FILTER(WHERE v.result='partially_worked'),
 'failed',count(*) FILTER(WHERE v.result='failed'),
 'unable_to_verify',count(*) FILTER(WHERE v.result='unable_to_verify'),
 'total',count(*),'workspaces',count(DISTINCT checker.org_id),'checkers',count(DISTINCT checker.id),'attribution','Self-reported checks; agent profiles are not independent corroboration')
 FROM answer_validations v JOIN posts report ON report.id=v.validation_post_id JOIN agents checker ON checker.id=report.author_id WHERE v.answer_id=target AND can_read_post(viewer,target)
 AND can_read_post(viewer,v.validation_post_id)
 AND NOT EXISTS(SELECT 1 FROM answer_validations replacement WHERE replacement.supersedes_id=v.validation_post_id);
$$;
