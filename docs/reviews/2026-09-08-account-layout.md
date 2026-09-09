# Account layout consistency

The account pages did not use the shared page shell. At a 1440px viewport, the landing heading began at x=88, the workspace heading at x=136, and the account heading at x=268 while its header brand began at x=48. On mobile, the workspace page used an 18px gutter while other public pages used 24px. The workspace brand and sign-out control also had independent sizing rules.

Both account templates now load the shared shell, and the workspace header reuses the shared brand and button components. All three pages now align their brand and heading at x=88 on a 1440px viewport and x=24 on a 390px viewport. Account forms retain a readable maximum width inside the shared shell.

The account stylesheet was consolidated around the rules actually used by templates and scripts. Removed unused selectors and duplicate overrides; active components use shared spacing, type, radius, color and control roles. The grep-backed count in human.css went from 12 raw color literals and 205 px values to zero of each. Fixed bright and inverse panels retain explicit foreground roles in both themes. Domain actions use a wrapping flex layout.

Evidence so far:

- 113 executed web tests passed against a fresh PostgreSQL database, no skips.
- Initial Chromium matrix: 280 rendered checks, zero contrast/accessibility/overflow failures.
- Supplemental audit: 112 text-spacing and CSS 200% zoom cases, no document overflow. CSS zoom is a rendering check, not a physical browser zoom or assistive-technology claim.
- Exact header and heading coordinates measured in Chromium at 320, 390, 1024, 1440 and 2880px.
- Final local Chromium/WebKit/Firefox matrix: 840 rendered checks, zero failures. Linux CI remains pending.

[Before](account-layout/workspaces-before-dark.png) and [after](account-layout/workspaces-after-dark.png) show the mobile dashboard. All fixture content is synthetic. These checks do not establish WCAG conformance.
