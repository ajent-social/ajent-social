#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
: "${AJENT_DNS_BROWSER_ADMIN_DSN:?Set an admin PostgreSQL URI on numeric loopback (127.0.0.1)}"

for command in createdb dropdb go npm python3; do
	command -v "$command" >/dev/null 2>&1 || {
		printf 'required command is missing: %s\n' "$command" >&2
		exit 2
	}
done

agent_browser_version=${AJENT_AGENT_BROWSER_VERSION:-0.38.2}
if [[ ! "$agent_browser_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	printf 'AJENT_AGENT_BROWSER_VERSION must be a pinned semantic version\n' >&2
	exit 2
fi

run_id=${AJENT_DNS_BROWSER_RUN_ID:-${GITHUB_RUN_ID:-local}_${GITHUB_RUN_ATTEMPT:-1}_$$}
suffix=$(printf '%s' "$run_id" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9_]+/_/g; s/^_+//; s/_+$//')
if [[ -z "$suffix" ]]; then
	printf 'could not create a database name from the run ID\n' >&2
	exit 2
fi
database="ajent_dns_browser_${suffix}"
if (( ${#database} > 63 )); then
	printf 'generated DNS browser database name exceeds PostgreSQL limit\n' >&2
	exit 2
fi

test_dsn=$(python3 - "$AJENT_DNS_BROWSER_ADMIN_DSN" "$database" <<'PY'
import sys
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

dsn, database = sys.argv[1:]
try:
    parsed = urlsplit(dsn)
    if parsed.scheme not in {"postgres", "postgresql"} or parsed.hostname != "127.0.0.1":
        raise ValueError("numeric loopback PostgreSQL URI required")
    if parsed.port is not None and not 1 <= parsed.port <= 65535:
        raise ValueError("invalid PostgreSQL port")
    query = [(key, value) for key, value in parse_qsl(parsed.query, keep_blank_values=True)
             if key not in {"dbname", "database"}]
    print(urlunsplit((parsed.scheme, parsed.netloc, "/" + database,
                      urlencode(query), parsed.fragment)))
except ValueError as exc:
    print(f"invalid AJENT_DNS_BROWSER_ADMIN_DSN: {exc}", file=sys.stderr)
    raise SystemExit(2)
PY
)

printf 'Creating fresh isolated DNS browser database %s on 127.0.0.1\n' "$database"
createdb --maintenance-db="$AJENT_DNS_BROWSER_ADMIN_DSN" --template=template0 "$database"
created=1
cleanup() {
	status=$?
	trap - EXIT
	if [[ "${created:-0}" == 1 ]]; then
		if ! dropdb --maintenance-db="$AJENT_DNS_BROWSER_ADMIN_DSN" --if-exists "$database"; then
			printf 'failed to drop disposable DNS browser database %s\n' "$database" >&2
			(( status != 0 )) || status=1
		fi
	fi
	exit "$status"
}
trap cleanup EXIT

cd "$repo_root"
npm exec --yes --package="agent-browser@${agent_browser_version}" -- agent-browser install
AJENT_TEST_DSN="$test_dsn" \
	npm exec --yes --package="agent-browser@${agent_browser_version}" -- \
	go test -tags=dnsbrowser ./internal/web -run '^TestBrowserMatchingDNSProof$' -count=1 -v
