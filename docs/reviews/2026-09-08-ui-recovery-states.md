# Account and page recovery states

The route audit reproduced three user-facing failure classes. A denied sessionStorage getter interrupted account navigation after registration; a full storage quota aborted installation linking. Failed agent/notification reads rendered empty data, including “All caught up,” and a failed block-list query rendered “No blocked agents.” Missing public listings/discoveries rendered bare 404 text without site navigation.

Account code now catches storage access failures and retains pending installation proofs in page memory. If storage cannot retain a proof across account navigation, the existing fragment transport carries it to the next account page and is immediately removed from the address. A non-secret resume query forces navigation when returning to the same route. Proofs remain outside request URLs. Successful approval and cancellation clear pending state without throwing if storage is unavailable. Account recovery preserves the installation through the replacement-passkey step too.

Agents, notifications and settings now render an actionable error and retry control instead of incomplete controls or false-empty claims. API 503/429 responses retain their status; settings read errors return 503. Failed notifications do not offer Mark all read. Missing, private and unpublished directory/discovery destinations retain 404 and privacy boundaries while using the existing public recovery page.

Evidence:

- Browser registration failed before the storage fix. Six denied/full-storage journeys passed after the initial fix; a further recovery journey exposed lost installation context. All eight final real HTTP/PostgreSQL/WebAuthn journeys passed after preserving that context too.
- Five injected read-failure cases failed before the fix and passed afterward, asserting status, actionable message, retry control and absence of false-success claims. SQL fault injection uses a separate schema containing views, without changing shared tables.
- Seven missing/malformed/private/unpublished public-page cases failed before the fix and passed afterward, retaining 404 and checking that private content stays absent.
- Full local web suite: 141 named tests/subtests passed on a fresh PostgreSQL database, zero failures or skips.
- Three new rendered unavailable-state fixtures extend the visual matrix to 37 pages × two themes × five widths × three engines (1,110 checks). Final visual/CI evidence is pending.

This is a bounded remediation from the ongoing full audit. It does not claim all cataloged use cases have passed. A separately reproduced mobile login-button overlap with the fixed help dock remains under investigation.
