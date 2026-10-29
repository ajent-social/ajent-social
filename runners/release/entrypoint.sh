#!/usr/bin/env bash
set -euo pipefail
# The task has no IAM task role. The execution role injects only this one-use
# GitHub runner configuration; workflow AWS credentials require main-ref OIDC.
[[ -n "${AJENT_JIT_CONFIG:-}" && "$AJENT_JIT_CONFIG" != disabled ]] || exit 64
# Keep the inherited secret out of subsequent job processes' environment.
jit=$AJENT_JIT_CONFIG
unset AJENT_JIT_CONFIG
# JIT runners execute one job and unregister. Bound idle and execution lifetime.
# The encoded config is ephemeral; never enable shell tracing here.
exec timeout --signal=TERM --kill-after=30s 10800 ./run.sh --jitconfig "$jit"
