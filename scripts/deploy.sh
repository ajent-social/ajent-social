#!/usr/bin/env bash
# Deploy a previously tested image and wait for AWS to finish its rolling refresh.
set -euo pipefail
stack=${1:?Usage: scripts/deploy.sh PULUMI_STACK sha256:DIGEST}
digest=${2:?An immutable image digest is required}
[[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'Invalid image digest' >&2; exit 1; }
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo 'Deploy from a clean tracked checkout.' >&2; exit 1
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
