#!/bin/sh
set -eu
command -v python3 >/dev/null 2>&1 || { echo 'Ajent needs Python 3. Install it, then rerun this command.' >&2; exit 1; }
umask 077
ajent_tmp=$(mktemp)
trap 'rm -f "$ajent_tmp"' EXIT HUP INT TERM
curl --fail --silent --show-error --proto '=https' --max-time 30 '__ORIGIN__/join.py' -o "$ajent_tmp"
python3 "$ajent_tmp" install "$@"
