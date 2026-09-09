#!/usr/bin/env bash
# Local, isolated qualification. No publication, deployment or installation edits.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(pwd -P)" == /private/tmp/ajent-shared-knowledge ]] || { echo 'Use the isolated shared-knowledge worktree.' >&2; exit 2; }
pgbin=/opt/homebrew/opt/postgresql@16/bin
claim=/Users/dndungu/.claude/skills/claim/scripts/claim.sh
output=/private/tmp/ajent-shared-verification
mkdir -p "$output"
load=$(uptime | sed -E 's/.*load averages?: ([0-9.]+).*/\1/')
awk -v load="$load" 'BEGIN { exit !(load <= 10) }' || { echo "Build held: load $load exceeds 10." >&2; exit 2; }
export CLAIM_REMOTE=/Users/Shared/mini-build-lease.git
lease_output=$("$claim" claim R-build-lease --purpose 'Ajent shared-knowledge local qualification')
printf '%s\n' "$lease_output"
[[ "$lease_output" == WON:* ]] || exit 2
lease_sha=${lease_output##* }
database="ajent_shared_verify_$(date +%s)_$$"
server_pid=''
cleanup() {
 if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
 if [[ -n "$lease_sha" ]]; then "$claim" release R-build-lease "$lease_sha"; fi
 "$pgbin/dropdb" -h 127.0.0.1 -p 55439 --if-exists "$database"
}
trap cleanup EXIT
[[ "$(git --git-dir="$CLAIM_REMOTE" rev-parse refs/claims/R-build-lease)" == "$lease_sha" ]] || exit 2
"$pgbin/createdb" -h 127.0.0.1 -p 55439 "$database"
export AJENT_TEST_DSN="postgres://$(id -un)@127.0.0.1:55439/$database?sslmode=disable"
go test -json -p 1 -race ./... -count=1 > "$output/go-tests.jsonl"
python3 - "$output/go-tests.jsonl" <<'PY'
import json,sys
rows=[json.loads(line) for line in open(sys.argv[1])]
passed=[r for r in rows if r.get('Action')=='pass' and 'Test' in r]
skipped=[r for r in rows if r.get('Action')=='skip' and 'Test' in r]
failed=[r for r in rows if r.get('Action')=='fail']
for package in sorted({r['Package'] for r in passed}):
 print(package, sum(r['Package']==package for r in passed), 'passed')
print(len(passed),'executed passes;',len(skipped),'test skips;',len(failed),'failures')
if not passed or skipped or failed:raise SystemExit(1)
PY
go vet ./...
go build -o "$output/server" ./cmd/ajentd
"$claim" release R-build-lease "$lease_sha"
lease_sha=''
python3 -m unittest discover -s internal/web/onboarding -p '*_test.py' > "$output/client-tests.log" 2>&1
export AJENT_DSN="$AJENT_TEST_DSN"
export AJENT_SETUP_TOKEN=synthetic-shared-knowledge-test-token-only
export AJENT_ORIGIN=http://localhost:55441
export AJENT_BROWSER_ORIGIN="$AJENT_ORIGIN"
for suite in group question feed; do
 export AJENT_BROWSER_OUTPUT="$output/browser/$suite"
 "$output/server" -addr 127.0.0.1:55441 > "$output/server-$suite.log" 2>&1 &
 server_pid=$!
 for attempt in {1..50}; do
  if curl -fsS "$AJENT_ORIGIN/healthz" > /dev/null 2>&1; then break; fi
  sleep 0.1
 done
 curl -fsS "$AJENT_ORIGIN/healthz" > /dev/null
 node "scripts/ui-$suite-journeys.mjs"
 kill "$server_pid"
 wait "$server_pid" || true
 server_pid=''
done
printf 'Evidence: %s\n' "$output"
