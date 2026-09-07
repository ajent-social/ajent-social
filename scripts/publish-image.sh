#!/usr/bin/env bash
# Run from the repository root after the foundation stack has been applied.
set -euo pipefail
stack=${1:?Usage: scripts/publish-image.sh PULUMI_STACK}
engine=${CONTAINER_ENGINE:-docker}
command -v "$engine" >/dev/null
revision=$(git rev-parse HEAD)
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo 'Release requires clean tracked files.' >&2
  exit 1
fi
region=$(pulumi -C infra config get aws:region --stack "$stack")
repository=$(pulumi -C infra stack output repositoryUrl --stack "$stack")
registry=${repository%%/*}
image="$repository:$revision"
release_dir=$(mktemp -d "${TMPDIR:-/tmp}/ajent-release.XXXXXX")
trap 'rm -rf "$release_dir"' EXIT
git archive "$revision" | tar -x -C "$release_dir"
"$engine" build --platform linux/arm64 --label "org.opencontainers.image.revision=$revision" -t "$image" "$release_dir"
bash "$release_dir/scripts/container-smoke.sh" "$image"
aws ecr get-login-password --region "$region" | "$engine" login --username AWS --password-stdin "$registry"
"$engine" push "$image"
repository_name=${repository#*/}
digest=$(aws ecr describe-images --region "$region" --repository-name "$repository_name" --image-ids "imageTag=$revision" --query 'imageDetails[0].imageDigest' --output text)
printf 'Verified image pushed. Configure imageDigest: %s\n' "$digest"
