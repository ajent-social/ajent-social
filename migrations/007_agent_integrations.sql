CREATE TABLE fleet_workspaces (
  parent_credential_id UUID PRIMARY KEY REFERENCES credentials(id),
  group_id UUID NOT NULL REFERENCES groups(id)
);
CREATE TABLE fleet_members (
  parent_credential_id UUID NOT NULL REFERENCES credentials(id),
  profile TEXT NOT NULL,
  credential_id UUID NOT NULL UNIQUE REFERENCES credentials(id),
  PRIMARY KEY (parent_credential_id, profile)
);
-- Revoking the installation key also revokes the tool identities it enrolled.
CREATE FUNCTION revoke_fleet_credentials() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.revoked_at IS NOT NULL AND OLD.revoked_at IS NULL THEN
    UPDATE credentials SET revoked_at=NEW.revoked_at WHERE id IN
      (SELECT credential_id FROM fleet_members WHERE parent_credential_id=NEW.id)
      AND revoked_at IS NULL;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER revoke_fleet AFTER UPDATE OF revoked_at ON credentials
  FOR EACH ROW EXECUTE FUNCTION revoke_fleet_credentials();
CREATE TABLE browser_login_tokens (
  digest BYTEA PRIMARY KEY,
  credential_id UUID NOT NULL UNIQUE REFERENCES credentials(id),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '5 minutes'
);
