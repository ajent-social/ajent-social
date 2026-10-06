#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
admin_dsn_input=${AJENT_DNS_BROWSER_ADMIN_DSN:?Set an admin PostgreSQL URI on numeric loopback (127.0.0.1)}
unset AJENT_DNS_BROWSER_ADMIN_DSN PGPASSWORD

for command in createdb dropdb go mktemp npm python3; do
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
install_agent_browser_browser=1
if [[ -n "${AGENT_BROWSER_EXECUTABLE_PATH:-}" ]]; then
	if [[ ! -x "$AGENT_BROWSER_EXECUTABLE_PATH" ]]; then
		printf 'AGENT_BROWSER_EXECUTABLE_PATH must name an executable browser\n' >&2
		exit 2
	fi
	install_agent_browser_browser=0
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

umask 077
private_dir=$(mktemp -d "$repo_root/.dns-browser-private.XXXXXX")
created=0
admin_dsn=
test_dsn=
passfile="$private_dir/pgpass"
# shellcheck disable=SC2317,SC2329
cleanup_dns_browser() {
	local status=$?
	trap - EXIT INT TERM
	if (( created )); then
		if ! dropdb --maintenance-db="$admin_dsn" --if-exists "$database"; then
			printf 'failed to drop disposable DNS browser database %s\n' "$database" >&2
			(( status != 0 )) || status=1
		fi
	fi
	if [[ -d "$private_dir" ]] && ! python3 -c 'import shutil,sys; shutil.rmtree(sys.argv[1])' "$private_dir"; then
		printf 'failed to remove private DNS browser credentials\n' >&2
		(( status != 0 )) || status=1
	fi
	exit "$status"
}
trap cleanup_dns_browser EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
chmod 700 "$private_dir"

AJENT_DNS_BROWSER_ADMIN_DSN="$admin_dsn_input" \
AJENT_DNS_BROWSER_PRIVATE_DIR="$private_dir" \
python3 - "$database" <<'PY'
import os
import sys
from pathlib import Path
from urllib.parse import parse_qsl, quote, unquote, urlencode, urlsplit, urlunsplit

dsn = os.environ["AJENT_DNS_BROWSER_ADMIN_DSN"]
private_dir = Path(os.environ["AJENT_DNS_BROWSER_PRIVATE_DIR"])
test_database = sys.argv[1]
try:
    parsed = urlsplit(dsn)
    if parsed.scheme not in {"postgres", "postgresql"} or parsed.hostname != "127.0.0.1":
        raise ValueError("numeric loopback PostgreSQL URI required")
    if parsed.port is not None and not 1 <= parsed.port <= 65535:
        raise ValueError("invalid PostgreSQL port")
    if parsed.fragment:
        raise ValueError("URI fragments are not supported")
    allowed = {"sslmode", "connect_timeout", "application_name", "dbname", "database"}
    query = parse_qsl(parsed.query, keep_blank_values=True)
    unknown = sorted({key for key, _ in query if key.lower() not in allowed})
    if unknown:
        raise ValueError("unsupported connection parameter(s): " + ", ".join(unknown))
    query = [(key, value) for key, value in query
             if key.lower() not in {"dbname", "database"}]
    username = unquote(parsed.username or "")
    password = unquote(parsed.password or "")
    admin_database = unquote(parsed.path.lstrip("/"))
    if not username or not admin_database:
        raise ValueError("URI username and maintenance database are required")
    port = parsed.port or 5432
    if any(char in username + password + admin_database for char in "\r\n\0"):
        raise ValueError("URI credentials and database may not contain control characters")

    authority = quote(username, safe="-._~") + "@127.0.0.1"
    if parsed.port is not None:
        authority += f":{port}"
    safe_query = urlencode(query)
    admin_uri = urlunsplit((parsed.scheme, authority, "/" + quote(admin_database, safe=""), safe_query, ""))
    test_uri = urlunsplit((parsed.scheme, authority, "/" + test_database, safe_query, ""))

    def pgpass_escape(value):
        return value.replace("\\", "\\\\").replace(":", "\\:")

    entries = []
    for database_name in (admin_database, test_database):
        entries.append(":".join(map(pgpass_escape, (
            "127.0.0.1", str(port), database_name, username, password
        ))))
    for name, value in (("admin-dsn", admin_uri), ("test-dsn", test_uri),
                        ("pgpass", "\n".join(entries) + "\n")):
        path = private_dir / name
        path.write_text(value)
        path.chmod(0o600)
except ValueError as exc:
    print(f"invalid AJENT_DNS_BROWSER_ADMIN_DSN: {exc}", file=sys.stderr)
    raise SystemExit(2)
PY
unset admin_dsn_input
admin_dsn=$(<"$private_dir/admin-dsn")
test_dsn=$(<"$private_dir/test-dsn")
export PGPASSFILE="$passfile"

printf 'Creating fresh isolated DNS browser database %s on 127.0.0.1\n' "$database"
createdb --maintenance-db="$admin_dsn" --template=template0 "$database"
created=1

cd "$repo_root"
if (( install_agent_browser_browser )); then
	npm exec --yes --package="agent-browser@${agent_browser_version}" -- agent-browser install
fi
artifact_dir=${AJENT_DNS_BROWSER_ARTIFACT_DIR:-$repo_root/artifacts/verification}
mkdir -p "$artifact_dir"
event_file="$artifact_dir/dns-browser-${suffix}.jsonl"
if AJENT_TEST_DSN="$test_dsn" \
	npm exec --yes --package="agent-browser@${agent_browser_version}" -- \
	go test -race -json -tags=dnsbrowser ./internal/web -run '^TestBrowserMatchingDNSProof$' -count=1 \
	| tee "$event_file"; then
	test_status=0
else
	test_status=$?
fi

validation_status=0
python3 - "$event_file" <<'PY' || validation_status=$?
import json
import sys
from pathlib import Path

test_name = "TestBrowserMatchingDNSProof"
roles = {f"{test_name}/{role}" for role in ("owner", "original", "human")}
try:
    events = [json.loads(line) for line in Path(sys.argv[1]).read_text().splitlines() if line]
except (OSError, json.JSONDecodeError) as exc:
    print(f"invalid Go JSON test artifact: {exc}", file=sys.stderr)
    raise SystemExit(1)

test_events = [event for event in events if event.get("Test")]
skipped = [event for event in events if event.get("Action") == "skip"]
failed = [event for event in events if event.get("Action") == "fail"]
runs = {event.get("Test") for event in test_events if event.get("Action") == "run"}
passed = {event.get("Test") for event in test_events if event.get("Action") == "pass"}
missing = sorted((roles | {test_name}) - runs)
unpassed = sorted((roles | {test_name}) - passed)
if not test_events or missing or unpassed or skipped or failed:
    print(
        "DNS browser verification requires the named suite and all three role "
        f"subtests to run and pass; tests={len(test_events)} missing_run={missing} "
        f"missing_pass={unpassed} skips={len(skipped)} failures={len(failed)}",
        file=sys.stderr,
    )
    raise SystemExit(1)
print(f"PASS {test_name} and {len(roles)} role subtests; zero skips and failures")
PY
if (( test_status != 0 )); then
	exit "$test_status"
fi
exit "$validation_status"
