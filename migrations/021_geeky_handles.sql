-- Profiles remain stable installation keys. Only generated display handles change.
CREATE TABLE agent_handle_history (
 agent_id UUID NOT NULL REFERENCES agents(id),
 old_handle TEXT NOT NULL,
 new_handle TEXT NOT NULL,
 changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(agent_id,old_handle)
);
CREATE FUNCTION geeky_agent_handle(workspace UUID, seed UUID) RETURNS TEXT LANGUAGE plpgsql AS $$
DECLARE
 first_words TEXT[] := ARRAY['cosmic','quantum','sleepy','spicy','wobbly','sneaky','disco','turbo','fuzzy','chaotic','crispy','neon','lunar','caffeinated','pixel','rubber'];
 last_words TEXT[] := ARRAY['debugger','kernel','daemon','duck','goblin','packet','byte','wizard','compiler','octopus','cache','noodle','robot','stack','gremlin','penguin'];
 base TEXT; candidate TEXT; n INTEGER := 0;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('geeky-handles:'||workspace::text,0));
 base := first_words[(('x'||substr(md5(seed::text),1,4))::bit(16)::integer % 16)+1] || '-' || last_words[(('x'||substr(md5(seed::text),5,4))::bit(16)::integer % 16)+1];
 candidate := base;
 WHILE EXISTS(SELECT 1 FROM agents WHERE org_id=workspace AND handle=candidate AND id<>seed) LOOP
  n := n+1;
  candidate := base||'-'||n::text;
 END LOOP;
 RETURN candidate;
END $$;
CREATE FUNCTION name_generated_agent() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE original TEXT;
BEGIN
 IF NEW.handle ~ '^(agent|installation|codex|claude-code|cursor|terminal|antigravity|opencode)(-[a-f0-9]{8})?$'
 AND (NEW.display_name=NEW.handle OR NEW.display_name='') THEN
  original:=NEW.handle;
  NEW.handle:=geeky_agent_handle(NEW.org_id,NEW.id);
  NEW.display_name:=NEW.handle;
 END IF;
 RETURN NEW;
END $$;
-- Preserve chosen names, agent UUIDs, credentials and fleet profile lookup keys.
DO $$
DECLARE a RECORD; new_name TEXT;
BEGIN
 FOR a IN SELECT id,org_id,handle FROM agents WHERE handle ~ '^(agent|installation|codex|claude-code|cursor|terminal|antigravity|opencode)(-[a-f0-9]{8})?$' AND (display_name=handle OR display_name='') ORDER BY created_at,id LOOP
  new_name:=geeky_agent_handle(a.org_id,a.id);
  INSERT INTO agent_handle_history(agent_id,old_handle,new_handle) VALUES(a.id,a.handle,new_name);
  UPDATE agents SET handle=new_name,display_name=new_name WHERE id=a.id;
 END LOOP;
END $$;
CREATE TRIGGER generated_agent_name BEFORE INSERT ON agents FOR EACH ROW EXECUTE FUNCTION name_generated_agent();
