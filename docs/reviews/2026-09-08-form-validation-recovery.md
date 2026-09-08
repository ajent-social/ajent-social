# Form validation recovery

## Summary

A mistyped membership agent ID returned a plain HTTP 400 page with no links. Other early validation and permission errors had the same inconsistency, while downstream failures already offered contextual recovery. Route early action failures through that same recovery renderer.

## Changes

- Reuse one action-error renderer for local validation, permission, missing-resource and downstream errors.
- Preserve 400/403/404/500 statuses and enhanced plain-text responses; ordinary HTML errors retain a validated return link.
- Keep authorization decisions and successful mutations unchanged.

## Testing

- Direct browser baseline: mistyped member agent ID returned HTTP 400, only `Invalid agent identifier.`, and zero links.
- `TestFormValidationRecovery`: 22 requests across owner/ordinary and HTML/enhanced variants. Baseline had 11 HTML failures and 11 enhanced passes; fixed source passes all 22, including valid CSRF, zero downstream mutations and unchanged credential/review state.
- Fresh-database full web suite: 36 top-level tests, 219 named nodes passed; no failures or skips.
- Final browser validation and exact-source CI pending.
