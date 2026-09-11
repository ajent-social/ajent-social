#!/usr/bin/env bash
# Deploy a previously tested image and wait for AWS to finish its rolling refresh.
set -euo pipefail
# The generated cloud SDK exceeds the shared mini's build budget. The Go
# program (including previews) runs on the authenticated infrastructure CI runner.
if [[ "${CI:-}" != "true" ]]; then
  echo 'Run Go infrastructure deployments in CI; local builds/previews are disabled.' >&2
  exit 1
fi
stack=${1:?Usage: scripts/deploy.sh PULUMI_STACK sha256:DIGEST}
digest=${2:?An immutable image digest or --migrate is required}
if [[ "$digest" == "--migrate" ]]; then
  baseline=${3:?Supply the private TypeScript no-change preview receipt}
  [[ -f "$baseline" ]] || { echo 'Missing TypeScript preview receipt.' >&2; exit 1; }
else
  [[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'Invalid image digest' >&2; exit 1; }
fi
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo 'Deploy from a clean tracked checkout.' >&2; exit 1
fi
# Refresh source ancestry before changing infrastructure. A stale parallel branch
# must integrate reviewed main changes rather than silently rolling them back.
git fetch origin main
if ! git merge-base --is-ancestor origin/main HEAD; then
  echo 'Release checkout must include current origin/main; reconcile before deployment.' >&2
  exit 1
fi
if [[ "$digest" != "--migrate" ]]; then
  migration=$(pulumi -C infra config get goMigrationRevision --stack "$stack" 2>/dev/null || true)
  if [[ ! "$migration" =~ ^[a-f0-9]{40}$ ]] || ! git merge-base --is-ancestor "$migration" HEAD; then
    echo 'Complete the separate --migrate no-op apply before changing the image.' >&2
    exit 1
  fi
fi
region=$(pulumi -C infra config get aws:region --stack "$stack")
group=$(pulumi -C infra stack output autoscalingGroup --stack "$stack")
[[ -n "$group" ]] || { echo 'Rolling fleet must be provisioned first.' >&2; exit 1; }
status=$(aws autoscaling describe-instance-refreshes --region "$region" --auto-scaling-group-name "$group" --query 'InstanceRefreshes[0].Status' --output text)
case "$status" in
  Pending|InProgress|Cancelling|RollbackInProgress|Baking) echo "Existing rollout is $status; wait before another deployment." >&2; exit 1 ;;
esac
before=$(aws autoscaling describe-instance-refreshes --region "$region" --auto-scaling-group-name "$group" --query 'InstanceRefreshes[0].InstanceRefreshId' --output text)
(cd infra && go vet ./... && go test ./...)
python3 -m unittest discover -s infra/tests -p '*_test.py'
if [[ "$digest" == "--migrate" ]]; then
  # Hold R-ajent-apply across baseline capture and this command. Keep the same
  # stack/configuration/image; the saved plan prevents changes during the apply.
  umask 077
  evidence=$(mktemp -d "${TMPDIR:-/tmp}/ajent-migration.XXXXXX")
  pulumi -C infra preview --stack "$stack" --refresh=false --json --show-sames --expect-no-changes --save-plan "$evidence/plan.json" > "$evidence/go-preview.json"
  python3 scripts/check-infra-migration.py "$baseline" "$evidence/go-preview.json"
  PULUMI_EXPERIMENTAL=true pulumi -C infra up --stack "$stack" --refresh=false --plan "$evidence/plan.json" --expect-no-changes --yes
  pulumi -C infra config set goMigrationRevision "$(git rev-parse HEAD)" --stack "$stack"
  printf 'No-op migration completed. Private receipts: %s\n' "$evidence"
  exit 0
fi
pulumi -C infra config set imageDigest "$digest" --stack "$stack"
pulumi -C infra preview --stack "$stack" --diff
pulumi -C infra up --stack "$stack" --yes
current=$(aws autoscaling describe-instance-refreshes --region "$region" --auto-scaling-group-name "$group" --query 'InstanceRefreshes[0].InstanceRefreshId' --output text)
if [[ "$current" != "$before" ]]; then
  python3 scripts/wait-rollout.py "$group" --region "$region"
else
  echo 'No new instance refresh; verify that the requested image is already deployed.'
fi
origin=$(pulumi -C infra stack output serviceUrl --stack "$stack")
curl --fail --silent --show-error --max-time 15 "$origin/healthz"
printf '\nVerify authenticated workflows before declaring the release complete.\n'
