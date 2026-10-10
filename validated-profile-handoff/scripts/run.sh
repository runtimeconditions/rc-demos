#!/bin/sh
# Reproducible local trust ceremony and two real upstream-validated handoffs.
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
demo=$(CDPATH= cd -- "$script_dir/.." && pwd)
repo=$(CDPATH= cd -- "$demo/.." && pwd)
catalog=${EXTENSIONS_ROOT:-"$repo/../extensions/catalog"}

"$repo/portable-profile/scripts/generate-profile.sh" --check
mkdir -p "$demo/generated"
run=$(mktemp -d "$demo/generated/run.XXXXXX")
trust=$(mktemp -d)
trap 'rm -rf "$trust"' EXIT INT TERM
mkdir "$run/bin"
(cd "$repo/portable-profile" && go build -o "$run/bin/dev-bind" ./cmd/dev-bind)
(cd "$demo" && go build -o "$run/bin/validate" ./cmd/validate)
(cd "$demo" && go build -o "$run/bin/consume" ./cmd/consume)
(cd "$demo" && go run ./cmd/keygen -out "$trust/keys")
# Consumer policy is deliberately outside both received bundles. Never copy the
# signing key into generated artifacts or CI uploads.
cp "$trust/keys/trusted.pub" "$run/trusted.pub"

"$run/bin/validate" -profile "$repo/artifacts/request-logger-http.profile.yaml" \
  -catalog "$catalog" -key "$trust/keys/signer.key" -out "$run/valid"
"$run/bin/consume" -bundle "$run/valid" -trusted-key "$run/trusted.pub" \
  -dev-bind "$run/bin/dev-bind" -env-out "$run/request-logger.env" > "$run/valid-result.json"
cat "$run/valid-result.json"

# A demand variant of the same workload, not a new application or a claim that
# request-logger's Redis client can talk to memcached. Validation is repeated
# upstream on this variant; the consumer is expected to decline it.
sed 's/engine: redis/engine: memcached/' "$repo/artifacts/request-logger-http.profile.yaml" > "$run/unsupported-input.yaml"
"$run/bin/validate" -profile "$run/unsupported-input.yaml" \
  -catalog "$catalog" -key "$trust/keys/signer.key" -out "$run/unsupported"
if "$run/bin/consume" -bundle "$run/unsupported" -trusted-key "$run/trusted.pub" \
  -dev-bind "$run/bin/dev-bind" -env-out "$run/must-not-exist.env" > "$run/unsupported-result.json"; then
  echo 'expected unsupported demand, but consumer accepted it' >&2
  exit 1
else
  status=$?
  [ "$status" -eq 3 ] || exit "$status"
fi
[ ! -e "$run/must-not-exist.env" ]
cat "$run/unsupported-result.json"
printf 'Inspect generated artifacts: %s\n' "$run"
