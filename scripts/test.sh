#!/usr/bin/env bash
set -euo pipefail
: "${AJENT_TEST_DSN:?Use a disposable PostgreSQL database; this suite mutates retention state}"
mkdir -p artifacts/verification
go test -json -p 1 -race ./... -count=1 | tee artifacts/verification/tests.jsonl
python3 - <<'PY'
import json
from pathlib import Path
entries=[json.loads(line) for line in Path('artifacts/verification/tests.jsonl').read_text().splitlines()]
passed=[e for e in entries if e.get('Action')=='pass' and 'Test' in e]
skipped=[e for e in entries if e.get('Action')=='skip' and 'Test' in e]
failed=[e for e in entries if e.get('Action')=='fail']
print(f'{len(passed)} test cases passed; {len(skipped)} skipped; {len(failed)} failed')
if not passed or skipped or failed: raise SystemExit('Verification requires executed tests with zero skips and zero failures')
PY
