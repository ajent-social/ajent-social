#!/usr/bin/env bash
# CodeBuild entry point: test an exact ARM64 release image before publishing.
set -euo pipefail
revision=${AJENT_REVISION:?AJENT_REVISION is required}
repository=${AJENT_REPOSITORY:?AJENT_REPOSITORY is required}
region=${AWS_REGION:-${AWS_DEFAULT_REGION:?AWS region is required}}
[[ "$revision" =~ ^[a-f0-9]{40}$ ]] || { echo 'Expected full commit SHA.' >&2; exit 1; }
[[ "$repository" =~ ^[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com/[a-z0-9][a-z0-9._/-]*$ ]] || { echo 'Expected ECR repository URL.' >&2; exit 1; }
[[ -f .ajent-release-revision && "$(cat .ajent-release-revision)" == "$revision" ]] || { echo 'Source revision marker mismatch.' >&2; exit 1; }
[[ "$(uname -m)" == aarch64 ]] || { echo 'Use a native ARM64 CodeBuild environment.' >&2; exit 1; }
registry=${repository%%/*}
repository_name=${repository#*/}
# A failed describe is not evidence that a tag is absent. Only ImageNotFound
# permits publication; IAM/network errors must stop the release.
error_file=$(mktemp)
trap 'rm -f "$error_file"' EXIT
if aws ecr describe-images --region "$region" --repository-name "$repository_name" --image-ids "imageTag=$revision" > /dev/null 2>"$error_file"; then
  echo 'Revision tag already exists; reconcile its evidence before reusing it.' >&2
  exit 1
elif ! grep -q '(ImageNotFoundException)' "$error_file"; then
  cat "$error_file" >&2
  exit 1
fi
mutability=$(aws ecr describe-repositories --region "$region" --repository-names "$repository_name" --query 'repositories[0].imageTagMutability' --output text)
[[ "$mutability" == IMMUTABLE ]] || { echo 'Release repository must enforce immutable tags without exclusions.' >&2; exit 1; }
image="$repository:$revision"
docker build --platform linux/arm64 --build-arg "AJENT_REVISION=$revision" --label "org.opencontainers.image.revision=$revision" -t "$image" .
bash scripts/container-smoke.sh "$image"
aws ecr get-login-password --region "$region" | docker login --username AWS --password-stdin "$registry"
docker push "$image"
echo 'Smoke-tested release image published.'
