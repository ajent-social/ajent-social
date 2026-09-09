# Help placement and login error visibility

At 390×844, the access-key sign-in button occupied y=765.55–814.34 while the fixed help dock began at y=772. Its center hit the dock instead of the button. A 320px regression reproduced the same obstruction. Bottom document padding did not reserve the dock's viewport area.

Help now occupies a normal footer row after page content. It remains keyboard-accessible and opens the same dialog, while no longer covering forms or other page controls. Cross-browser pointer testing then exposed a WebKit focus defect: clicking the launcher did not focus it, so closing help left focus in the closed dialog. The dialog now remembers the actual activating control, with a launcher fallback for automatic opening.

The real invalid-key screenshot also exposed a dark-mode contrast defect: `.card p` overrode `.error`, producing muted text on a bright error background (1.62:1). The existing error color pair now wins within cards (7.51:1). The invalid access-key field is associated with the existing alert, and the error state is included in visual fixtures.

Evidence:

- The initial pointer assertion failed with the dock intercepting the visible button.
- WebKit failed focus restoration before the activating-control fix; an independent close-event probe confirmed focus remained on the closed dialog.
- A real-server invalid-key contrast assertion failed at 1.62:1 before the selector fix.
- All nine final real-server pointer/error/focus journeys passed: Chromium, WebKit and Firefox at 320, 390 and 1024px. They assert unobscured content, real invalid-key submission, error association and contrast, launcher/close interaction, restored focus and no horizontal overflow. They make zero chat API requests.
- All nine existing help journeys passed, including draft retention, cancellation, retry and source navigation.
- All 1,140 local visual checks passed: 38 fixtures × two themes × five widths × three engines. The final alert ID association was subsequently checked in the real-server pointer suite; it does not change visual layout. Exact-source Linux CI remains pending.

[Final mobile screenshot](help-footer/mobile-dark.png) uses a synthetic invalid key and contains no account credentials.
