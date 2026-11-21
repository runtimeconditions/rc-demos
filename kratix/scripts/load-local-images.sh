#!/usr/bin/env bash
# Build the demo images under the names the manifests use and load them into a
# KinD cluster. Run the other scripts with IMAGE_PULL_POLICY=IfNotPresent so
# the cluster uses these images instead of pulling from ghcr.io.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
KIND_CLUSTER="${KIND_CLUSTER:-rc-demos}"
REGISTRY="ghcr.io/runtimeconditions"

build() {
  printf '[platform-demo] building %s\n' "$1"
  docker build -q -t "${REGISTRY}/$1:latest" "${@:2}"
}

build application-release-pipeline "${REPO_ROOT}/kratix/promises/application-release/pipeline"
build redis-pipeline "${REPO_ROOT}/kratix/promises/redis/pipeline"
build todos-api "${REPO_ROOT}/apps/todos-api"

build request-logger "${REPO_ROOT}/apps/request-logger-http"

for image in application-release-pipeline redis-pipeline todos-api request-logger; do
  kind load docker-image "${REGISTRY}/${image}:latest" --name "${KIND_CLUSTER}"
done

printf '[platform-demo] local images loaded into kind cluster %s\n' "${KIND_CLUSTER}"
