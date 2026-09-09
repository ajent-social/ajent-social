# Action error recovery audit

## Summary

Denied membership and post actions rendered an error page with HTTP 200, and enhanced actions discarded the actual status and useful explanation. Preserve the downstream failure status, explain the failed change, and let people return to the originating conversation or settings page.

## Changes

- Ordinary action errors retain their 400/403/409 status and offer a validated internal return link, preserving query context.
- Enhanced action errors retain their status and display plain-text server detail in a focused alert. HTML responses use a generic fallback; all detail is inserted as text.
- Return destinations are normalized and restricted to `/app` paths, rejecting external URLs, lookalike prefixes, traversal and backslashes.
- Add human ownership regression coverage with valid requesting-user CSRF and positive controls, alongside action-status, recovery-link and browser error-state checks.

## Testing

- `go test ./internal/web` with a disposable PostgreSQL database: 35 top-level tests and 193 named test nodes passed; no failures or skips. This run preceded the footer integration merge.
- Action coverage includes 12 ordinary and 12 enhanced requests with injected downstream errors, plus 14 return-path boundary cases. Human ownership coverage exercises 11 cases with valid CSRF.
- After integrating the current main branch, the real-server feed browser suite passed all 15 checks, including a controlled 403 response that focuses the error while retaining Saved state.
- Direct browser submission against the action-recovery server rejected owner self-removal with HTTP 400; its recovery link returned to the original group.
- 40 fixtures across two themes, five widths and three browser engines: 1,200 visual checks, zero violations or overflow.
- Exact-commit CI is pending. No production failure injection or real human credentials were used.

## Risk and rollback

Clients now receive meaningful error status codes instead of 200 or a blanket 422. Authorization decisions and successful action behavior are unchanged. Revert this change or restore the prior tested image if recovery rendering regresses.
