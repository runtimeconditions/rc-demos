# Published catalog and validation evidence

This demo hands off a generated Runtime Conditions Profile with signed validation
evidence to an independent platform consumer. See [README.md](README.md) for the
full trust model, acceptance contract, and Kratix integration.

## Pinned tools and definitions

| Component | Version |
| --- | --- |
| Official `rc-cli` | `v0.6.0` (bundled Go profiler `v0.8.0`, core schema `0.4.0`) |
| Published Go common/env bindings | `v0.3.0` |
| Common/env extension definitions | `v1alpha2` |
| Extension resolver module | `v0.0.0-20261005083147-3cc942274975` |
| Handoff Go toolchain | `1.27.4` |

[`portable-profile/scripts/install-rc.sh`](../portable-profile/scripts/install-rc.sh)
installs the official CLI with archive SHA-256 verification. `RC_CLI` can
select an explicit executable.

## Published catalog versus offline fixtures

**Normal path:** The producer retrieves these versioned published definitions
over HTTPS, resolving their transitive dependencies:

- https://runtimeconditions.io/extensions/rc/common-integrations/v1alpha2/runtimeconditions.extension.yaml
- https://runtimeconditions.io/extensions/rc/env-configuration/v1alpha2/runtimeconditions.extension.yaml

The producer snapshots the exact response bytes and validates the **unchanged**
Profile against that private snapshot with
`rc profile validate - --catalog <snapshot>`. It signs evidence only after
validation succeeds. Evidence binds canonical extension identities and digests
of the actual validated bytes; a retrieval or validation failure **never falls
back** to a local definition or alternate version.

**Explicit offline test mode:** The checked-in
[`internal/producer/testdata`](internal/producer/testdata/README.md) YAML files
are unchanged copies of published HTTPS responses for deterministic dependency,
failure-path, byte-integrity, and offline acceptance tests. They are **not a
private catalog or the normal source of extension definitions**. Offline mode
is selected explicitly with `-catalog DIR` or, in the test harness,
`EXTENSIONS_ROOT`. The normal path continues to use published URLs.

The consumer independently verifies the configured signer and byte digests,
then evaluates platform support without repeating RC semantic validation.
The evidence format is experimental, not a new RC schema, execution attestation,
or production signing system.

## Reproduce

From the repository root, with Go 1.27.4:

```sh
export RC_CLI="$(sh portable-profile/scripts/install-rc.sh)"
portable-profile/scripts/generate-profile.sh --check

# Published HTTPS catalog (network access required)
(cd validated-profile-handoff && go test -v ./...)

# Explicit offline fixture acceptance
(cd validated-profile-handoff && EXTENSIONS_ROOT=internal/producer/testdata go test -v ./...)

python3 -m unittest discover -s validated-profile-handoff/kratix -p 'test_*.py' -v
validated-profile-handoff/scripts/run.sh
portable-profile/scripts/run-dev.sh

# Requires Docker, KinD, kubectl, and PyYAML:
validated-profile-handoff/scripts/kind-ci.sh
```

The five handoff cases cover trusted evidence, changed Profile bytes,
changed extension bytes, an untrusted signer, and valid-but-unsupported
`memcached` demand. KinD/Kratix is a separate real-cluster check; local
process-level success is not evidence of a cluster pass.
