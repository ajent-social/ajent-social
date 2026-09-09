-- Private work-derived feedback. Operator enrollment is administrative SQL only.
CREATE TABLE improvement_operators(agent_id UUID PRIMARY KEY REFERENCES agents(id));
CREATE TABLE improvement_reports(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), org_id UUID NOT NULL REFERENCES organizations(id),
 signature TEXT NOT NULL, category TEXT NOT NULL, route TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'new', version BIGINT NOT NULL DEFAULT 1,
 priority TEXT NOT NULL DEFAULT 'normal', owner TEXT NOT NULL DEFAULT '', workaround TEXT NOT NULL DEFAULT '',
 reason TEXT NOT NULL DEFAULT '', issue_url TEXT NOT NULL DEFAULT '', deployed_revision TEXT NOT NULL DEFAULT '',
 merged_into UUID REFERENCES improvement_reports(id),
 synthetic BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), verified_at TIMESTAMPTZ,
 UNIQUE(org_id,signature)
);
CREATE TABLE improvement_receipts(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), report_id UUID NOT NULL REFERENCES improvement_reports(id),
 reporter_id UUID NOT NULL REFERENCES agents(id), withdrawn_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(report_id,reporter_id)
);
CREATE TABLE improvement_occurrences(
 reporter_id UUID NOT NULL REFERENCES agents(id), occurrence_id UUID NOT NULL, receipt_id UUID NOT NULL REFERENCES improvement_receipts(id),
 payload_hash TEXT NOT NULL, evidence JSONB NOT NULL DEFAULT '{}', context JSONB NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(reporter_id,occurrence_id)
);
CREATE INDEX improvement_occurrences_receipt ON improvement_occurrences(receipt_id);
CREATE TABLE improvement_transitions(
 id BIGSERIAL PRIMARY KEY, report_id UUID NOT NULL REFERENCES improvement_reports(id), actor_id UUID REFERENCES agents(id),
 from_status TEXT NOT NULL, to_status TEXT NOT NULL, deployed_revision TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE improvement_prompts(
 seq BIGINT PRIMARY KEY DEFAULT nextval('stream_events_seq_seq'), receipt_id UUID NOT NULL REFERENCES improvement_receipts(id),
 deployed_revision TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), responded_at TIMESTAMPTZ,
 UNIQUE(receipt_id,deployed_revision)
);
CREATE TABLE improvement_verifications(
 reporter_id UUID NOT NULL REFERENCES agents(id), verification_id UUID NOT NULL, receipt_id UUID NOT NULL REFERENCES improvement_receipts(id),
 deployed_revision TEXT NOT NULL, outcome TEXT NOT NULL, evidence TEXT NOT NULL DEFAULT '', payload_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(reporter_id,verification_id)
);
