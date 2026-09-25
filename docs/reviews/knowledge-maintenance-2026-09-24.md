# Knowledge candidate maintenance — 2026-09-24

Candidate code `c1887a1` consolidates PRs #60, #61, #62 and #66 into #68 and
merges current service main `eaf184e`. All four superseded heads are ancestors
of this candidate. Current goals, compact MCP delivery, pagination and public
copy are preserved. The route-inventory tripwire now counts 68 agent routes.

## Corrected defects

- **P1: private outbox starvation.** The initial brain selector admitted idle
  indexed rows and terminal operation conflicts that the locked query could
  never process. It could keep returning the same idle brain, preventing work
  for another brain. Apply the actionable-row predicate at both selection
  stages and order due work consistently. Both table-driven regression cases
  failed before the fix and passed afterward under the race detector.
- **P1: private-only global knowledge panic.** Group-only configuration creates
  a Service without a global BrainID or Backend. Reliability advertised global
  knowledge and global search dereferenced the nil backend. Global capability
  advertisement and all four global handlers now require the configured global
  partition and backend. The regression reproduced the false advertisement
  and actual panic before the fix; all cases pass afterward.

## Verification

Fresh disposable PostgreSQL 16 database, shared build lease held only during
qualification:

- `go test -json -race -p 1 ./... -count=1`: 555 test/subtest pass records,
  zero failures, zero skips.
- `go vet -p 1 ./...`: pass.
- `go build -p 1 ./...`: pass.
- Python onboarding discovery: 84 tests passed.
- `git diff --check`: pass.

This remains a draft candidate. No merge to main, deployment, canonical
publication, production two-brain proof or Go-client cutover was performed.
The canonical Go client/discovery work continues in `ajent-social/ajent`;
the Python compatibility surface remains until its E8 cutover is qualified.
