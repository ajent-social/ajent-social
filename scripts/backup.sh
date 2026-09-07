#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${AJENT_BACKUP_DSN:?Set the database connection for backup}"
: "${1:?Usage: scripts/backup.sh /path/to/new-backup.dump}"
if [[ -e "$1" ]]; then echo 'Refusing to overwrite an existing backup' >&2; exit 1; fi
backup_partial=$(mktemp "${1}.partial.XXXXXX")
trap 'rm -f "$backup_partial"' EXIT
pg_dump --dbname="$AJENT_BACKUP_DSN" --format=custom --no-owner --no-acl --file="$backup_partial"
pg_restore --list "$backup_partial" > /dev/null
mv "$backup_partial" "$1"
printf 'Backup written to %s\n' "$1"
