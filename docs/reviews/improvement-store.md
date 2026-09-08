# Durable improvement store verification

T30.3 and T30.4 storage primitives, 2026-09-08. Migration 015 stores unique
repository/report/operation/version keys, safe marker-only intents, database-time
leases and generation fencing. It deliberately has no report foreign key so
unresolved external-effect tombstones survive private report retention.

`Store.MarkSentTx` lets the worker validate locked control/report rows in the same
transaction as the sent intent. The transaction must commit before external I/O.
Only pending preflight failures can retry. Sent outcomes are reconciled by exact
marker, with five-minute spacing after ambiguity. Five claims exhaust the bounded
attempt budget; unresolved ambiguous records stay durable for operator attention.
Withdrawal cancels pending jobs and fences prior owners; already sent intents stay
ambiguous. Completed, cancelled and dead history expires after 90 days in batches
of at most 1,000 rows.

Validation on a disposable PostgreSQL database: six named tests plus two subtests
passed under the race detector, zero skipped or failed. Each test uses its own
schema. Concurrent insertion, simultaneous claimants, same-owner takeover, expired
renewal, late completion, intent rollback, withdrawal, retry delay/cap, exact-repo
isolation, marker validation and retention are covered. Removing generation
comparison made `TestStoreFencing` fail with `late response=<nil>`; restoring the
check passed. Scoped `go vet` and formatting passed.

Execution used a fresh Kazi proposal (`prop-ajent-durable-store`): capability
predicates were red at t0 and environment/format guards green. The first executor
was stopped before convergence after review found contract mismatches; the lead
approved direct review correction. No Kazi convergence claim is made. The executor
also ran one broad build contrary to the scoped-build instruction; it completed
before inspection and was not repeated. Final validation was single-package only.

This is local storage evidence, not deployment, live cutover, alert delivery, or
the required 24-hour production observation gate.
