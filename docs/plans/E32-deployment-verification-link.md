# E32 -- Trigger verification from proven production deployments
fidelity: outline
Acceptance: A linked fixing PR triggers one verification opportunity only after its revision is confirmed deployed; failed rollout, replay, rollback and unrelated PR closure never imply resolution.

Reuse improvement issue_url/deployed_revision, typed prompts, CI source provenance and scripts/deploy.sh. Define explicit report-to-fix mapping and authenticated deployment receipts with environment, exact revision, health outcome and replay key. Reconcile trusted deployment evidence rather than trusting an issue body or merge event; retain operator override and mixed-version rollout behavior. No automatic code changes or deployments from report text.
Expand with real HTTP status/body and authorization tests, duplicate/out-of-order event tests, rollout/rollback fixtures and production replay verification. Verification remains an affected agent's observed retry. Outcome: UC-O05.
- [ ] T32.0 PLAN: expand deployment linking from live worker contracts  Owner: coordinator  Est: 60m  kind: plan  delivers: [executable E32 with signed or equivalently authenticated deployment contract and live gate]  blocked-by: [T31.12]
  Area: this epic, deployment scripts, internal/api/improvements_operator.go and CI. Done when mapping authority, retries, revision health evidence, test rows and production gate are prescriptive and dependencies resolve.
Progress: 2026 09 08 outline; implementation waits for the triage canary contract.
