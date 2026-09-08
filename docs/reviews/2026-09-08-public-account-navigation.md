# Public navigation for signed-in accounts

Public pages always displayed Sign in, even with a valid human or agent session. The account browser journey reproduced this immediately after passkey registration: the guide/home navigation had no return link to workspaces. Agent sessions were sent toward the separate human sign-in flow.

A shared navigation script now checks a same-origin, non-cacheable session-status response. Valid human sessions get Your workspaces; agent-only sessions get Agent dashboard; signed-out browsers retain Sign in. The response contains exactly two booleans and uses the existing protected-page session validation. No names, IDs, credentials or CSRF values are returned. Human navigation takes precedence if both session types are present. The connection action remains available to signed-in visitors, and space is reserved for changing labels.

Navigation refreshes on page restoration, focus and tab visibility. Failed or malformed status responses leave the usable return link in place. No cross-origin session endpoint or periodic polling was introduced.

Validation:

- The missing endpoint initially failed all four session-state cases. Those cases now pass, covering anonymous, human, expired and invalid cookies. The existing owner web flow also checks agent-only navigation state.
- All 124 web tests passed against a fresh PostgreSQL database, with no skips.
- Ten real-server account journeys passed, including human return links across seven public pages at 320px and 390px. Fourteen feed journeys passed, including agent dashboard return links.
- The cross-tab journey changes real sessions in one page and dispatches a focus event to the other. Both headed and headless automation reported both pages visible/focused and emitted no activation events, so native OS tab activation is not claimed as verified.
- 1,020 local visual checks passed: 34 named production-template fixtures × two themes × five widths × three engines. The new authenticated fixture states use a controlled boolean response; real authentication is covered by the separate browser journeys.
- Final Linux CI remains pending.

Synthetic navigation screenshots: [human mobile](public-navigation/human-mobile-dark.png), [agent desktop](public-navigation/agent-desktop-light.png). These are browser-engine checks, not a WCAG conformance or physical-device claim.
