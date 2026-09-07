# Mobile usability — September 7, 2026

The mobile release preserves the frameless glass chat from PR #4 and incorporates
PR #5's connection branding repair and original design review. It fixes the
public header collision and hidden links, replaces the clipped app navigation
with a native Menu & account disclosure, and restores mobile identity/sign-out.
The disclosure works without JavaScript; Escape closes it when enhanced.

Shared rules reserve scrollable clearance below page content for the launcher,
wrap forms, credentials and long names, retain audience labels, expose footer
links, and keep input text at 16px. Orange text uses #b83c08, measuring 5.39:1
against #fff8eb, while button fills and artwork retain the bright orange.
The chat follows the visual viewport to fit above mobile keyboards; bubbles
remain frameless and fade upward. No verification renewal rules changed.

## Executed verification

- Chromium: 104 route/viewport checks passed. Nine public routes at
  320/390/700/1024/1440/2880px; ten authenticated owner routes at
  320/390/700/1024/1440px. Checked overflow, public navigation visibility and
  brand collisions, all mobile account destinations, keyboard dismissal,
  form text sizing, and final-page action clearance from the launcher.
- Fresh disposable PostgreSQL database with explicitly labelled demo content;
  additional local credential with a long handle. No production mutation or
  credential screenshots. Mobile sign-out was exercised and subsequent app
  navigation required login. Zero browser JavaScript errors in the matrix.
- Short 390×420 viewport: chat input visible, close/reopen worked and the draft
  remained. No-JavaScript mobile login, full account menu and sign-out passed.
- Targeted Go Ask/validation tests passed; shared mobile assets appear once in
  every complete HTML template; git diff --check passed.

Local evidence: /private/tmp/ajent-mobile-evidence. Browser runners:
/private/tmp/check-ajent-mobile.cjs and
/private/tmp/check-ajent-mobile-keyboard.cjs. They use the local test database
and preview origin, not production accounts.

These are Chromium viewport tests, not physical iPhone/Safari certification or
a full screen-reader audit. The page can scroll past the fixed launcher;
opening chat deliberately overlays the underlying page. The broader design
review's domain-verification flow, raw error pages and dark-mode work remain
separate. Deployment requires the assigned infrastructure owner's clear and
R-ajent-apply lease; merging this change does not establish it is live.
