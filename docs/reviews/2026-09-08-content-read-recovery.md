# Content read recovery

## Summary

Feed, group, profile and conversation read failures could return HTTP 200, display empty-state claims, or omit useful recovery controls. Show a consistent recovery page with the actual failure status and preserve query parameters when retrying.

## Changes

- Preserve failed shared-profile/group and content-read statuses, including 404, 429 and 503.
- Hide incomplete content and creation controls behind the shared recovery state, avoiding false empty-group/feed claims.
- Skip dependent content reads after an earlier required read fails.
- Preserve search, filter and cursor query values in the GET retry form.
- Give transient conversation failures retry wording instead of suggesting expiry; retain generic missing-resource wording for unavailable content.

## Testing

- Expanded `TestReadFailureProbes` reproduced six additional failures against the baseline: feed 503/429, search 503, group list 503, missing profile 404 and conversation 503. All now pass alongside the five existing cases.
- Fresh-database `go test ./internal/web -count=1 -json`: 35 top-level tests and 206 named nodes passed, no failures or skips. Checks include preserved retry query fields and Retry-After for rate limits.
- Added four rendered recovery fixtures; exact-source browser/visual CI is pending.

## Risk and rollback

Unavailable content now returns meaningful error statuses instead of HTTP 200. This affects ordinary and enhanced read requests; existing enhanced-error handling preserves prior content and focuses its error alert. Authorization and successful reads are unchanged. Revert this change or restore the prior tested image if recovery rendering regresses.
