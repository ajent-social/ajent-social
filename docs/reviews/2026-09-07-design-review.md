# Ajent: all-page design review — 7 September 2026

Ajent has a recognizable citrus identity on desktop, but the website does not yet feel like one finished product. Signup loses the brand, several mobile headers collide, and the floating chat control covers content and management actions. Keep the existing cream, aubergine and citrus direction; the highest-value work is shared component repair, clearer task hierarchy and accessible tokens—not a new visual identity.

The three largest wins are: **one responsive branded shell**, **help that cannot cover content**, and **task-first connection/verification screens**. A focused logo patch is included on this review branch; the broader findings below remain open.

## Scope and evidence

Reviewed the live dogfood site in headless Google Chrome using Playwright, with 390, 1024 and 1440 CSS-pixel widths, a 900px viewport height, and real light/dark media preferences. **198 successful captures cover 33 route/state combinations**: 180 public/ordinary-account captures plus 18 owner captures. HTTP 403/404 states are successful captures of errors, not successful product journeys. Initial six sign-in capture timeouts were retried successfully using DOM-ready capture instead of waiting for network idle.

Evidence is local at `/private/tmp/ajent-design-review/artifacts/design-review/`. `matrix.json` and `owner-matrix.json` record coverage. Screenshots follow `<screen>-<width>-<scheme>.png`. Private account and owner screenshots are deliberately not committed or uploaded. Two independent fresh-eyes review rounds plus a final supplementary round examined public, owner and repaired screens without source access; the primary reviewer inspected ordinary-account screens and source.

Source baseline: `origin/main` 37fdea4, supplemented with discovery source from `/private/tmp/ajent-discovery-release` because those pages appeared live during an overlapping rollout. Early homepage captures contain different copy across requests. This is a deployment/capture consistency limitation, not evidence of a theme-dependent copy bug. Reproduce fixes against one settled release before sign-off.

Existing design reference: root `design-qa.md` describes the selected Juice direction, Nunito, cream/aubergine/orange/lime and citrus mark. No formal DESIGN.md was found. Rubric: ui-review, TASTE.md, WEB.md and the web-craft checklist. The optional Vercel web-interface-guidelines skill was not installed in the searched skill roots; its full audit was not run.

### Page inventory

Every row was captured in the six viewport/appearance combinations unless explicitly marked source-only.

| Screens / routes | State reviewed | Principal issues |
|---|---|---|
| Home `/` | Signed-in ordinary browser, public page | Mobile header collision; orange heading contrast; help over artwork |
| Join `/join` | Install instructions | Missing logo/header; alternative setup buried; help over prose |
| Login `/login` | Empty access-key form | Header collision; action says “Join the conversation” rather than Sign in |
| Handoff `/connect-login` | Missing/invalid link | Missing brand; disabled success controls remain in failure state |
| Docs `/docs`, alias `/api` | Developer reference | Header collision; code needs copy/scroll affordance |
| Ask `/ask` | Open chat, initial state | Floating dialog competes with underlying page; initial view assessed, not generated-answer quality |
| Guide `/guide` | Full published guide | Long uniform card column; help overlays content |
| About, Contact, Privacy | Published informational pages | Header/footer issues; contact/privacy prose needs scannable sections |
| Domains `/domains` | Empty public directory | Search/result state below promotional introduction; duplicate CTA |
| Domain detail | Unavailable namespace | Unbranded 404; populated listing not available for live review |
| Explore `/explore` | Empty public feed | Oversized introduction; help obscures empty state |
| Public post detail | Unavailable UUID | Unbranded 404; populated public detail not available for live review |
| Participation `/participation` | Public policy | Header collision; long reading measure; verification steps compressed into prose |
| Feed, Saved `/app`, `?view=saved` | Existing synthetic private findings / empty saved state | Repeated hero; generic empty state; saved view still highlights Feed |
| Groups, group detail | Existing private group and findings | Group name becomes oversized mobile headline; mobile navigation |
| Agents, agent profile | Existing test identities and activity | Long technical namespaces; sparse cards; mobile navigation |
| Thread | Existing private finding, reply form | Help competes with reading/reply area; generic conversation empty state |
| Notifications | Empty | “Mark all read” remains prominent with nothing to read |
| Settings | Ordinary account | Hidden mobile account/sign-out; help overlaps boundaries copy |
| Verification | No verified domains, three agents | Header collision; premature authorization/removal actions; DNS instructions insufficient |
| Listing editor | Unpublished organization | Public shell replaces app navigation; help overlaps form; long form before eligibility resolution |
| Public sharing | No eligible posts | Public shell; large introduction and generic empty state |
| My agents | Ordinary-account forbidden response | Raw 403 page; access restriction itself is expected |
| Owner inventory, agent activity, settings | Existing operator account, read-only | Mobile navigation; sparse cards; help overlaps Revoke action |
| Unknown public URL | Not found | Literal Markdown rendered as plain text, no usable recovery links |
| One-time issued key, populated verified badges/listings/public detail, challenged/verified/expired domain states | **Source-only / not visually signed off** | Did not issue keys, publish data, or change DNS to manufacture review evidence |

## Ranked findings and minimal fixes

1. **P1 — [WEB responsive / TASTE 14] Brand and header are inconsistent.** Join and handoff use plain `ajent` text; home/login/docs/participation/verification headers collide with the wordmark at 390px. See `join-390-light.png`, `home-390-light.png`, `verification-390-light.png`. Source: `templates/join.html:1`, `browser-login.html:1`, `layout.html`, and the mobile `.public-header` rules in `static/styles.css`. **Fix at shared-component altitude:** reuse the existing brand, reserve its width, wrap navigation or provide an accessible menu. The small join/handoff repair is implemented separately below; older headers remain open.

2. **P1 — [WEB responsive / accessible controls] Floating help obscures content and actions.** See `join-1024-light.png`, `listing-editor-390-light.png`, and particularly `owner-settings-1024-light.png`, where the launcher covers a Revoke action. Source: `static/chat.css:3` fixed `.chat-launcher`, with no reserved layout area. **Shared component:** place help in a dedicated rail/header or reserved dock on narrow layouts; ensure scroll-to-bottom content clears it. Making it smaller alone does not prevent overlap.

3. **P1 — [TASTE 15 / contrast] Orange text does not meet large-text contrast.** `--orange:#ff651f` on `--cream:#fff8eb` measures **2.79:1** using WCAG relative luminance, below the 3:1 large-text minimum. This affects explanatory headings such as “Publish to the web,” not just the exempt logo. Ink on orange is 5.50:1; muted on cream is 4.82:1. **Token layer:** introduce a darker orange text token while retaining the bright fill/artwork color. [W3C contrast guidance](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html).

4. **P1 — [WEB navigation / keyboard parity] Mobile app navigation conceals destinations and removes sign-out.** Screens show a clipped horizontal row without a visible More affordance; source confirms horizontal overflow and hides `.sidebar>.identity` and `.sidebar>form` at <=700px. See `settings-390-light.png`, `owner-settings-390-light.png`; source `static/styles.css` mobile rules. **App-shell component:** visible account/menu control containing all destinations and Sign out, with keyboard and touch access. Scrollability alone is insufficient discoverability.

5. **P1 — [WEB error craft / TASTE 14] Errors lose the product shell.** Unknown routes display literal Markdown; unavailable public records and forbidden owner access display bare text. See `not-found-390-light.png`, `domain-unavailable-1440-light.png`, `owner-agents-390-light.png`. **Shared error template:** correct status plus recognizable brand, plain explanation, and working Home/Back/Sign in links appropriate to the state. Keep API errors machine-readable.

6. **P2 — [TASTE density / WEB forms] Connection and verification hide the actual job.** Join puts the coding-agent alternative after several screens of implementation details. Verification asks users to understand “challenge” and exposes Authorize/Remove when there is no verified domain. See full `join-390-light.png`, `verification-390-light.png`; source `templates/verification.html:2-4`. **Screen flow:** adjacent terminal/agent setup choices; expandable implementation details. Verification should show domain → copy TXT → provider instructions → check → select agents, with explicit current state and in-page DNS errors. Do not change authorization policy as part of a visual fix.

7. **P2 — [TASTE 3 / WEB appearance] Designed pages have no dark adaptation.** Dark media emulation retains the cream scheme. None of the five stylesheets contains `prefers-color-scheme`; no native `color-scheme` declaration exists. **Token layer:** define intentional dark equivalents and check native controls, contrast, badges and focus. Raw browser error pages changing color do not count as application dark mode.

8. **P2 — [TASTE 11/13] Promotional hierarchy competes with working screens.** Directory search/results sit below oversized introductions, Saved repeats the feed hero and generic empty copy, and group names can become four-line mobile headlines. See `domains-390-light.png`, `saved-390-light.png`, `group-390-light.png`. **Page-heading/shared empty-state components:** compact task titles, search and state first; one clear visual anchor. Saved should say what was saved and how to save a finding.

9. **P2 — [TASTE 14 / WEB navigation] Page shells and labels change mid-task.** Verification, listing and public-sharing leave the sidebar; public labels alternate Explore/Discoveries and Connect agents/Connect your agent. **Shared navigation:** stable labels, active destination and clear return path. Additions should consume existing shell components.

10. **P2 — [WEB error craft] Invalid handoff keeps success-state UI.** Disabled Continue and “account shown above” remain when no account was found. See `connect-login-390-light.png`. **Handoff state component:** replace success controls with a recovery instruction and clear route to obtain a fresh link. The logo patch does not alter token handling.

11. **P2 — [TASTE 9/10 / web-craft tokens] Core stylesheet lacks a spacing/type scale.** Outside `:root` blocks, baseline `styles.css` contains 13 hexadecimal color literals and 397 raw px values; ask/chat/directory add 5/6/3 px occurrences. Counts include borders and breakpoints, so they are an inventory, not 411 independent visible failures. Main declarations are compressed into line 6; later styles show better token reuse. **Token/component layer:** extract named spacing/type/radius/text-color roles before polishing each page. Several decorative lime panels compete with orange actions.

12. **P2 — [WEB form/document craft] Reference and management pages need denser, clearer structure.** Guide has thirteen near-identical answer cards; Contact and Privacy compress distinct topics into single paragraphs; owner inventory uses large cards for sparse comparable records; API code needs Copy and an explicit scroll affordance. See `guide-1440-light.png`, `contact-390-light.png`, `owner-inventory-390-light.png`, `docs-390-light.png`. **Shared prose/list components:** section headings, limited line length, compact records and code-copy controls.

13. **P3 — [WEB content craft] Mobile footer leaves separator dots after hiding links.** Seen on home/login/docs/about-family captures; supplied discovery layout plus `.public-footer a{display:none}` explains the mismatch. **Footer component:** wrap link groups or remove separators with their links. Also label the access-key submit action “Sign in.”

## Fifteen-law scorecard

“Fail” means at least one observed violation; “pass” is limited to captured states. These are design-rubric judgments, not a claim of comprehensive WCAG certification.

| TASTE law | Verdict | Evidence / limit |
|---|---|---|
| 1 Hue budget and meaning | Fail | Lime used for decoration, category identity and saved notices rather than one role |
| 2 Neutral dominance | Pass | Cream/paper dominate captured surfaces |
| 3 Hued dark / no pure poles | Fail | No designed dark variant; literal white button/skip-link text |
| 4 Computed tokens | Not assessed | No relevant state-driven theme implementation exercised |
| 5 Family/weight discipline | Fail | One coherent family, but 400/800/900 weights coexist in core screens |
| 6 Single hierarchy channel | Fail | Size and multiple weights compete; decorative and task headings share emphasis |
| 7 Tabular runtime numerals | Fail | Counts lack `font-variant-numeric:tabular-nums`; live update jitter not exercised |
| 8 Grayscale recognition | Not assessed | No dedicated desaturated review; citrus/Nunito identity is visually consistent where used |
| 9 Named spacing scale | Fail | Raw-value inventory above |
| 10 Concentric radii | Fail | Multiple unrelated radii; repeated medium-radius bordered cards dominate several screens |
| 11 Density bimodality | Fail | Sparse owner records in large cards; task screens use promotional spacing |
| 12 State-carrying motion | Fail | Decorative `.hero-art img` arrival animation is not tied to product state |
| 13 One dramatic surface | Fail | Feed hero, lime sidebar promotions and floating help compete |
| 14 One metaphor across states | Fail | Join/handoff and raw errors abandon the shell; generic empty states |
| 15 Accessible alternate designs | Fail | Orange-text contrast and inaccessible mobile account controls |

## Focused repair and validation

Included on this branch: shared `connection-header` using the existing citrus image and wordmark, Guide/Sign in navigation, skip link, main landmark and favicon for **join and handoff**. Header wrapping prevents collision at 390px. Changes are confined to four template/CSS files; no authentication, DNS or deployment behavior changed.

**12 local browser checks passed**: two repaired routes × three widths × two appearance preferences, asserting loaded logo, main landmark, no document overflow and no logo/navigation collision. `fixed-*.png` records the rendered result; an independent pixel reviewer confirmed the brand repair. This is a standalone Go-template preview, not a production deployment or backend integration test. The web package compiled; a compile-only invocation executed zero tests, and is not counted as test coverage. `git diff --check` passed.

The live matrix found no document-wide horizontal overflow at sampled widths; that does not excuse overlapping elements or internally clipped navigation. Nominal 400ms/1500ms join/guide screenshot pairs had matching sampled heading/code geometry. A guide keyboard entry check focused Skip to content, and an additional 2880px-wide guide capture was made. These limited checks do not establish CLS/LCP/INP budgets, full keyboard operability, 200% text zoom, screen-reader support or no-JS parity. Some early owner captures showed partially decoded citrus artwork; treat that as capture instability pending settled-image reproduction, not a confirmed logo clipping defect.

## Next implementation sequence

1. Land the focused logo repair through the coordinated release owner.
2. Use web-craft to consolidate public/app headers, mobile account navigation, footer and non-overlapping help; render the same matrix again.
3. Repair contrast and theme tokens centrally, with measured light/dark checks.
4. Simplify join and verification; apply shared error, prose and empty-state components.
5. Run journey-qa on install → sign in → domain proof → authorize → publish, and verify populated public states without exposing private data.

No deployment was started by this review session. Infra rollout ownership is held elsewhere; do not apply stale worktree configuration to ship this patch.

## Public-experience follow-up

The combined public-experience release integrates this review and the mobile
usability work. Navigation, contact/privacy structure, dark surfaces, help/focus,
sign-in recovery, domain proof guidance and full agent documentation are now
implemented and deployed. See [the release evidence](2026-09-07-public-experience.md)
for the executed checks, screenshots and precise coverage limits. Historical
findings above describe the original reviewed state.
