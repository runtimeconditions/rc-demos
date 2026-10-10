#!/usr/bin/env bash
# Run only against the dedicated disposable cluster; never the current context.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEMO="$ROOT/validated-profile-handoff"
export KIND_CLUSTER=validated-profile-handoff
export IMAGE_PULL_POLICY=IfNotPresent
OUT="$DEMO/generated/kratix"
if [[ -e "$OUT" ]]; then
  echo "Output already exists: $OUT; use a fresh checkout/output directory" >&2
  exit 1
fi
if kind get clusters | grep -Fxq "$KIND_CLUSTER"; then
  echo "Refusing to reuse existing cluster $KIND_CLUSTER" >&2
  exit 1
fi
mkdir -p "$DEMO/generated"
config_dir="$(mktemp -d)"
export KUBECONFIG="$config_dir/kubeconfig"
cleanup() {
  code=$?
  python3 "$DEMO/kratix/assert_cluster.py" --collect-only --diagnostics "$OUT/diagnostics" || true
  kind delete cluster --name "$KIND_CLUSTER" || true
  rm -f "$config_dir/kubeconfig"
  rmdir "$config_dir" || true
  exit "$code"
}
trap cleanup EXIT
kind create cluster --name "$KIND_CLUSTER" --wait 120s
"$ROOT/portable-profile/scripts/generate-profile.sh" --check
python3 "$DEMO/kratix/prepare.py" --out "$OUT"
"$ROOT/kratix/scripts/01-install-kratix.sh"
"$ROOT/kratix/scripts/load-local-images.sh"
for image in verifier resolver; do
  docker build -t "rc-handoff-$image:experiment" -f "$DEMO/kratix/Dockerfile.$image" "$ROOT"
  kind load docker-image "rc-handoff-$image:experiment" --name "$KIND_CLUSTER"
done
# Only the Redis prerequisite and the opt-in Promise, not the ungated original.
sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/' "$ROOT/kratix/manifests/promises/redis.yaml" | kubectl apply -f -
kubectl wait --for=create crd/redis.platform.demoteam.io --timeout=120s
kubectl wait --for=condition=Established crd/redis.platform.demoteam.io --timeout=120s
kubectl apply -f "$ROOT/kratix/manifests/catalog/todos-api-catalog.yaml"
# Default Kratix resource workflows run in the resource request namespace.
kubectl -n demo create configmap handoff-trust --from-file=trusted.pub="$OUT/trusted.pub"
python3 "$DEMO/kratix/promise.py" > "$OUT/promise.yaml"
kubectl apply -f "$OUT/promise.yaml"
kubectl wait --for=create crd/validatedapplicationreleases.platform.demoteam.io --timeout=120s
kubectl wait --for=condition=Established crd/validatedapplicationreleases.platform.demoteam.io --timeout=120s
kubectl apply -f "$OUT/requests"
python3 "$DEMO/kratix/assert_cluster.py" --diagnostics "$OUT/diagnostics"
