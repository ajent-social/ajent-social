# Populated-page and cross-browser UI audit

The expanded audit found 36 horizontal-overflow failures in populated feeds, profiles, and threads at 390px and 1024px across Chromium, WebKit, and Firefox. Long verified domains were rendered with the generic pill class, so the existing `.verified-badge` wrapping rule did not apply. The badge also placed an anchor inside the author anchor, causing browser HTML repair to move markup outside its intended container.

The badge now uses its wrapping class and is a separate link beside the author block. The block and its text can shrink inside the post card. No identity is truncated or hidden.

| Check | Before | After |
| --- | --- | --- |
| Original expanded matrix: 27 pages × 2 themes × 4 widths × 3 engines | 36 failures / 648 checks | 0 failures / matched 648 checks |
| Added 320px coverage | Not executed in that baseline | 162 checks, 0 failures |
| Initial local matrix | — | 810 checks, 0 failures |
| Final local matrix: 28 pages × 2 themes × 5 widths × 3 engines | — | 840 checks, 0 failures |

The runner checks exact fixture membership, successful responses, nonzero executed contrast checks, WCAG 2/2.1 A/AA axe rules, and horizontal overflow. It renders production templates with synthetic data. Populated cases include feeds, profiles, threads, directories, settings, long workspace/account names and public errors. Screenshot capture preserves viewport dimensions for overflowing pages instead of silently expanding the viewport to fit the defect.

Before and after examples: [overflowing domain](ui-expansion/domain-before-dark.png), [wrapped domain](ui-expansion/domain-after-dark.png).

This is automated coverage, not a claim of WCAG conformance or physical Safari/Firefox device testing. WebKit is the Playwright engine. Virtual passkey and feed journeys run separately against the production server and PostgreSQL.

A grep-backed token audit found remaining literals outside the shared `styles.css` token file: `human.css` has 12 color literals and 205 px values; `chat.css` has 22 and 58; the other stylesheets have no color literals and 16 px values combined. Some values define scoped chat tokens; account styles also contain legacy rules. This count is design-system debt, not evidence of 34 contrast failures. The rendered contrast failures from the original account report were repaired separately in PR 34.

Linux CI exposed five additional failures: native verification buttons in dark WebKit had 1.68:1 contrast. Those controls now use the shared button classes, verification forms wrap on narrow screens, and a compact domain-tools navigation replaces the duplicate brand header. A populated verification fixture opens disclosure controls so those actions are included. The final local matrix passes all 840 checks; the revised Linux CI run is pending.
