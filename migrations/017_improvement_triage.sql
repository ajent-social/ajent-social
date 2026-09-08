-- Decisions contain bounded enums, hashes and references, never reporter text.
CREATE TABLE improvement_decisions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 repository TEXT NOT NULL REFERENCES improvement_control(repository),
 report_id UUID NOT NULL REFERENCES improvement_reports(id) ON DELETE CASCADE,
 report_version BIGINT NOT NULL,
 policy_version BIGINT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 mode TEXT NOT NULL CHECK(mode IN ('shadow','canary')),
 state TEXT NOT NULL DEFAULT 'processing' CHECK(state IN ('processing','proposed','applied','rejected','stale')),
 proposal JSONB NOT NULL DEFAULT '{"action":"needs_review","priority":"normal","evidence_indexes":[]}',
 fingerprint TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT 'collecting',
 synthetic BOOLEAN NOT NULL DEFAULT false,
 reserved_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK(reserved_micro_usd>=0),
 observed_micro_usd BIGINT CHECK(observed_micro_usd>=0),
 processor_started BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(repository,report_id,report_version,policy_version)
);
CREATE INDEX improvement_decisions_review ON improvement_decisions(repository,id);
CREATE TABLE improvement_triage_budget (
 repository TEXT NOT NULL REFERENCES improvement_control(repository),
 day DATE NOT NULL,
 reserved_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK(reserved_micro_usd>=0),
 PRIMARY KEY(repository,day)
);
CREATE TABLE improvement_decision_reviews (
 id BIGSERIAL PRIMARY KEY,
 decision_id UUID NOT NULL REFERENCES improvement_decisions(id) ON DELETE CASCADE,
 actor_id UUID NOT NULL REFERENCES agents(id),
 action TEXT NOT NULL CHECK(action IN ('accept','change','reject')),
 decision_version BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
