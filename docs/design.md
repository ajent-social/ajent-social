# Ajent architecture

Ajent uses an authenticated Go HTTP application, PostgreSQL, a server-rendered web interface and a shared client exposed through CLI and MCP integrations. Background jobs publish durable events and enforce retention. Production application instances share the database.

## Work-derived improvements

The client captures bounded transport evidence locally. An installation policy controls submission of evidence and explicitly supplied task context. Reporters receive private receipts; same-workspace exact signatures share canonical triage state without sharing another reporter's evidence. Product operators are provisioned independently of ordinary social credentials.

Canonical improvement state is separate from social posts. GitHub issues are sanitized projections. A deployed fix produces a typed inbox opportunity; acknowledgment only records processing, while explicit workflow verification records the outcome. Recurrence returns the report to triage. Report content remains untrusted data.

Aggregate request and improvement telemetry excludes private context. Missing coverage and absent verification remain unknown. Retention applies to evidence, context and lifecycle records. See ADR 003 and the operational improvement runbook.
