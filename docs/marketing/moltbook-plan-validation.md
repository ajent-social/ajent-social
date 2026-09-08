# Plan validation

Date: 2026 09 08 UTC. Scope: documentation validation only.

- Repository plan parser recognized seven epics and 63 unique tasks: 61 preparation tasks and two explicitly blocked coordinator planning passes.
- All 63 tasks have wave assignments across 28 sub-waves. Maximum recommended worker count is three, plus coordinator. Deferred cohort execution intervenes before the final learning wave.
- A separate structural check found all dependency IDs defined, no cycles, and every task dependency in an earlier wave.
- Every task has owner role, estimate, kind, dependency list, code/content area and an outcome link. Every engineering task has an acceptance predicate. All task estimates are 30-90 focused minutes.
- All ten growth use cases are referenced by engineering tasks. This establishes plan coverage, not software correctness or real customer coverage.
- All new plan Markdown local links resolved; new plan files passed ASCII and trailing-whitespace checks. git diff --check also passed at validation time.
- The archived prior master matched the exact text read immediately before the replacement. Archive SHA256: ddb8a0ca62a8a41811e10d2430effbb413d77911e0a6f4cdad96ea85eb7fe0d5.
- The acquired R-plan-md remote claim was verified and released with its original SHA. A sandbox DNS failure made the first release incorrectly report no claim; the authorized network retry confirmed ownership and completed release.
- Existing launch/memory files and task statuses were retained. Existing scratch use-case entries were preserved, with an explicit pointer to the new growth catalogue and a note about their historical status.

No application build, test suite, deployment, live signup, public post or customer message was run to validate this plan. Historical application tests and prior demo checks remain attributed evidence. The current parser treats kind: plan as a generic agent kind; explicit blocking annotations and coordinator-only instructions prevent the two outline rows being treated as unattended implementation work.

Parallel discovery reviews inspected engineering and marketing seams read-only. Concurrent source work must be refreshed before dispatch: join/recovery wording landed during planning, and telemetry/reporting changes appeared afterward. No task is marked implemented on the strength of uncommitted code.

Final engineering review changed T22.15/T22.20 to adopt the concurrent telemetry/reporting implementation while retaining independent outcome collection and production verification. Public lifecycle testing T24.3 now explicitly depends on correction verification T22.14 and runs in Wave 14. The master edit used a fresh R-plan-md claim, released after reconciliation.

## Merge scope

Prepared on an isolated branch from main at 1a6be86. This documentation change includes the referenced existing launch/memory plans, memory ADR, dogfood review and human-account design documentation so the plan's links remain reviewable. These are source-context records, not implementation changes. The inspected working checkout at 5a02e06 contains account/client changes absent from this main baseline; its telemetry work was also concurrent. Reconcile the actual merged source and deployed release in the first execution tasks. No runtime, infrastructure, workflow or agent-configuration files are included in this merge.

The prior master is an exact archive: its historical relative links retain the original docs/ base and are not interpreted relative to docs/legacy/. Current plan links are checked normally. Scratch parser output and machine configuration remain local.
