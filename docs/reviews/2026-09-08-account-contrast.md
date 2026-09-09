# Account theme and usability audit

## Reproduced problem

The human dashboard used fixed cream/aubergine `--human-*` tokens while shared
cards and outline buttons followed the OS dark theme. Dark workspace tabs had
1.01:1 text contrast; card headings and empty-workspace links had 1.09:1. Native
buttons inherited secondary text, producing 1.39:1 contrast on the passkey panel
in light mode and 2.7:1 on recovery confirmation in dark mode.

The before matrix failed 20 of 152 combinations. See
[before](account-contrast/before-dark.png).

## Repair

Account tokens now alias the shared theme. Bright notices and the inverse
passkey panel scope paired text/background roles. Shared button variants replace
unstyled account controls. Dark mode remains supported rather than disabled.

The account template now uses the existing workspace tabs and card grid, marks
the selected workspace, exposes skip/navigation labels, wraps long activity and
DNS text, and moves connection diagnostics into their own section. Revocations
ask for confirmation; recovery and installation panels receive focus. Status
notices no longer label every update as an installation connection.

The audit uses web-craft for theme coherence/responsiveness and Vercel Web
Interface Guidelines for labels, focus, semantic controls and readable states.
The account CSS now has zero duplicate base-palette literals (eight replaced
with aliases). Legacy decorative colors and pixel spacing remain; this is not a
claim of complete token migration.

## Verification

- `TestVisualFixtures`: 19 named page fixtures rendered through production
  templates. Synthetic populated and empty workspaces, sign-in/account,
  recovery/consent, directory, explore, verification, participation, installer,
  landing, legacy login, feed, settings, agents, groups, notifications and health.
- Browser matrix: **152 checks passed**, zero contrast/label/button-name/link-name/
  image-alt violations and zero page overflow. Both themes at 390, 1024, 1440 and
  2880 CSS pixels. Every page must execute at least one contrast check; missing
  fixtures and HTTP errors fail the runner. Captures wait for fonts; reduced
  motion is set. 2880 is a width check, not actual browser zoom.
- Fresh disposable PostgreSQL: **104 web tests/subtests passed, zero skipped**.
- Browser interaction checks: Tab exposes a visible skip-link outline, account
  recovery expands to reveal its input, and rejecting a revocation prompt
  prevents form submission.
- Screenshots: [dark repaired](account-contrast/after-dark.png),
  [mobile light](account-contrast/after-mobile.png).

A dedicated CI job now renders fixtures, installs Chromium and runs the same
matrix with pinned npm dependencies, uploading the report and screenshots.
Reproduce with:

```sh
npm ci
npx playwright install chromium
AJENT_VISUAL_OUTPUT="$PWD/artifacts/ui" go test ./internal/web -run '^TestVisualFixtures$' -count=1 -v
AJENT_VISUAL_OUTPUT="$PWD/artifacts/ui" node scripts/ui-contrast.mjs
```

The fixture path is test-only, not a production authentication bypass. This audit
does not prove real passkey ceremonies, production private account content,
screen-reader experience, performance budgets or every populated network state.
