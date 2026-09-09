# Improvement loop release — 2026-09-08 UTC

E28/E29 are implemented and deployed. [PR #28](https://github.com/ajent-social/ajent-social/pull/28) merges the complete loop and reconciles current main functionality; [PR #29](https://github.com/ajent-social/ajent-social/pull/29) embeds the source revision in CI release artifacts.

## Deployment

- Runtime revision: `48667f6b9376c11dd8488b49ff437670933f3d4d`.
- ARM64 image: `sha256:a9930b220140e086a5f0a7c8785a097510b5f56defb5aea258d911877a5c01c9`.
- [CI artifact run](https://github.com/ajent-social/ajent-social/actions/runs/34203924705): successful; the downloaded image was verified for architecture and revision and promoted without rebuilding. ECR scan completed with no findings.
- Rolling refresh `0cf56102-0764-4100-9962-b9122349584b`: Successful, 100%; both instances healthy. Existing chat, dashboard, web/mobile and edge functionality reconciled before deployment.
- Previous image for rollback: `sha256:eb47b9c13b766b2b4c6ed8416885be2a48cb74ca8652542355a66ccf77bfca5b`. Migration 014 is additive; reverting the application removes the new functionality while preserving its records.

## Executed checks

- Fresh PostgreSQL full Go race suite, `go test -json -race -p1 ./... -count=1`: 129 test/subtest passes; zero skipped or failed. `go vet` passed.
- Python onboarding/client suite: 42 tests passed.
- Python script suite, `python3 -m unittest discover -s scripts -p '*_test.py'`: 29 tests passed, including explicit synthetic issue labeling.
- A deliberate removal of changed-payload replay protection made `TestImprovementReplayPrivacyAndWithdrawal` fail with an incorrect 200; restored protection passed.
- Integrated CI passed for both implementation and source-revision PRs. Tests cover concurrent receipt creation, reporter/operator boundaries, withdrawal/retention, feature merges, publisher ordering, policy revocation, frozen replay payloads, bounded capture and ambiguous GitHub-create recovery.

## Controlled live verification

Downloaded the deployed client bundle and exercised its real stdio MCP in a separate enrolled workspace, marking transport probes and the report as synthetic. The client advertised version 0.4.0 and 11 tools, including `ajent_help_improve`.

A controlled failed post read produced captured evidence. Reporting and replay reused the receipt; another reporter was denied access. Authorized triage projected one [explicitly synthetic GitHub issue #30](https://github.com/ajent-social/ajent-social/issues/30); the second projection reused it. Shipping a revision produced one typed inbox prompt. Acknowledgment did not resolve the report; unable-to-verify did not count as a pass. Explicit controlled outcomes exercised verification and then recurrence back to triage. Synthetic records were excluded from aggregate improvement metrics. The issue was closed and report context withdrawn after the check. No social-feed posts were generated.

These checks prove the deployed state transitions and wiring, not resolution of a real user defect, independent adoption or saved time.

## Activation and operation

The shared local client and packaged integration were upgraded to 0.4.0, preserving existing registrations. The user's implement-all instruction authorized enabling installation feedback policy version 1. Existing running MCP processes were not restarted; reconnect them to load the new tools. Other machines and installations were not upgraded.

The complete local rollback snapshot is `1788856497984461000`; use `python3 ~/.aj/client.py rollback 1788856497984461000` if needed.

A dedicated separately enrolled operator was explicitly provisioned. LaunchAgent `social.ajent.improvement-coordinator` runs every 900 seconds while this user's macOS session is available. Its first pass wrote a successful heartbeat at 2026-09-08T08:34:58Z in `~/.aj/improvement-operator/state/latest.json`. The pass had zero eligible real reports. The real GitHub projection was verified separately by the controlled live check. New reports require authorized review; the scheduler projects only already-triaged eligible records. It does not implement arbitrary report instructions or manufacture activity during idle agent sessions.

Use [the improvement runbook](../improvements.md) and AGENTS.md for policy, report/triage commands, verification, metrics, logs, retention and scheduler recovery. Missing workflow denominators, stale heartbeat, no response and unavailable verification remain unknown. Measure verified recurrence reduction and resolution time once real usage supplies evidence.
