# Public visitor → human + agent activation

Approved by David: “Plan it and do it,” 2026-09-08.

## Plan

1. Replace the abstract homepage pitch with a concrete shared-fix benefit,
   one primary `/start` CTA, privacy reassurance and a labeled example exchange.
   Keep the existing orange identity. Point empty discovery states to the example.
2. Add an account-first `/start` route using the existing passkey ceremony.
   Continue to `/join` after recovery-code acknowledgment; installation approval
   links must retain precedence. Lead setup with a copyable agent prompt and
   collapse terminal details. Preserve existing installation and consent behavior.
3. Show server-derived account/installation progress in the human workspace,
   with an explicit first private finding prompt and a refresh action. Never
   count a copied prompt as connection or claim a useful outcome from a request.
4. Verify real account registration, recovery acknowledgment, installation
   approval and private post retrieval against a disposable database. Check public
   pages and onboarding at 390, 1024, 1440 and 2880 CSS pixels, keyboard and
   clipboard failure states. Record named suites, executed counts and limitations.

## Evidence and measurement

Live review on 2026-09-08: homepage “More juice. Together”; `/join` leads with
terminal setup; account creation is under sign-in; `/domains` and `/explore`
show empty states. The example must be labeled illustrative, never live activity.

The recent usage report covers 2026-09-07 18:59Z–2026-09-08 18:59Z. It contains
request counts, mixed releases and unattributed traffic, so it cannot establish
unique visitor conversion. No uplift claim is justified. Desired outcome is a
human with an approved installation whose agent performs a useful first action.
Actual usefulness needs workflow confirmation; posts alone are not proof.
No visitor identifiers or new tracking cookies will be introduced by this change.

## Scope and release

Preserve pre-existing working-tree changes. No fabricated network activity,
automatic public posting, DNS requirement for private use, or changes to other
installed clients. Keep installation approval and human/agent credential isolation.
Record implementation and verification below before release.

## Implementation

- Added `/start`, account-aware setup, server-derived installation readiness and
  a first private finding prompt. The panel collapses once a finding is shared.
- Reworked the homepage and empty discovery states around a labeled exchange.
- Preserved current main’s form recovery, storage-denial handling, mobile shell,
  chat widget and account navigation when integrating the initial implementation.
- Added `TestConversionJourneyStates` and a real browser conversion suite in CI.

## Verification in progress

The initial implementation passed 111 Go test/subtest records under the race
  detector, with zero skips or failures, plus `go vet ./...` and a native build.
A real local browser against a disposable PostgreSQL database passed 38 checks:
passkey registration, recovery acknowledgment, setup prompt copying and denial,
installation consent, two-agent private retrieval and responsive page layouts.
An expired-installation regression was observed failing before its fix.

These results preceded integration onto current main; final integrated evidence
will be recorded after CI. Local full builds are held while the mini exceeds
the shared build policy’s load threshold. Conversion uplift remains unmeasured.
