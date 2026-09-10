-- Durable external-effect ownership. No reporter context or report foreign key:
-- unresolved marker tombstones must outlive report/context retention.
CREATE TABLE improvement_jobs (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 repository TEXT NOT NULL CHECK(length(repository)<=201),
 report_id UUID NOT NULL,
 operation TEXT NOT NULL CHECK(length(operation) BETWEEN 1 AND 128),
 version_key TEXT NOT NULL DEFAULT '' CHECK(length(version_key)<=128),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN('pending','sent','ambiguous','done','cancelled','dead')),
 attempts INT NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 next_attempt TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 owner TEXT NOT NULL DEFAULT '',
 generation BIGINT NOT NULL DEFAULT 0,
 lease_until TIMESTAMPTZ,
 marker TEXT NOT NULL DEFAULT '' CHECK(length(marker)<=128),
 issue_url TEXT NOT NULL DEFAULT '' CHECK(length(issue_url)<=512),
 reason TEXT NOT NULL DEFAULT '' CHECK(reason ~ '^[a-z0-9_]{0,64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(repository,report_id,operation,version_key)
);
CREATE INDEX improvement_jobs_due ON improvement_jobs(repository,next_attempt,created_at,id) WHERE state IN('pending','sent','ambiguous');
CREATE INDEX improvement_jobs_lease ON improvement_jobs(lease_until) WHERE lease_until IS NOT NULL;
CREATE INDEX improvement_jobs_retention ON improvement_jobs(updated_at) WHERE state IN('done','cancelled','dead');
