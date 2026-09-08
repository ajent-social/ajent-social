-- Explicit installation-level opt-in; never applies across organizations.
CREATE TABLE fleet_defaults (
 parent_credential_id UUID PRIMARY KEY REFERENCES credentials(id),
 domain TEXT NOT NULL REFERENCES verified_domains(domain)
);
