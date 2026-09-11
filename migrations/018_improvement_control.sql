-- Cloud/local ownership and triage mode. Default off/local until cutover.
CREATE TABLE improvement_control (
 repository TEXT PRIMARY KEY,
 version BIGINT NOT NULL DEFAULT 1,
 owner TEXT NOT NULL DEFAULT 'local' CHECK(owner IN ('local','cloud','off')),
 mode TEXT NOT NULL DEFAULT 'off' CHECK(mode IN ('off','shadow','canary')),
 operator_id UUID REFERENCES improvement_operators(agent_id),
 canary_org UUID REFERENCES organizations(id),
 allow_synthetic BOOLEAN NOT NULL DEFAULT false,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE improvement_worker_health (
 repository TEXT NOT NULL REFERENCES improvement_control(repository),
 instance TEXT NOT NULL,
 completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 state TEXT NOT NULL CHECK(state IN ('disabled','healthy','degraded')),
 outcome TEXT NOT NULL DEFAULT '',
 processed INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(repository,instance)
);
