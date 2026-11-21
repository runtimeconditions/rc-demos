# Published catalog and validation evidence after PR #10

This contribution is rebased onto upstream main `17b0f6b701f91c6ecbaeac262b6f3cec2b96ac05`,
including merged [PR #10](https://github.com/runtimeconditions/rc-demos/pull/10).
It retains the catalog and evidence work requested in
[the review discussion](https://github.com/runtimeconditions/rc-demos/pull/10#issuecomment-7064729451).

## Scope

PR #10 supplies the Go, Java, and Python published bindings, typed declarations,
workload images, canonical Profile, optional Condition names, and Java/Python
build checks. Those implementations are unchanged here. The handoff envelope
retains #10's rule: names are optional and uniqueness applies only to names
that are present. Diagnostic labels never rewrite Profile bytes.

This follow-up adds the producer's HTTPS catalog snapshot and released CLI
validation, signs their exact-byte identities, and keeps the downstream
consumer free of RC semantic validation. It removes sibling profiler and
extension checkout requirements from the handoff workflow. Both generation
and validation use the same checksum-pinned official CLI release.

## Published dependencies

| Component | Pin |
| --- | --- |
| Official `rc-cli` archive | `v0.6.0` |
| Bundled Go profiler | `v0.8.0` |
| Embedded core schema | `0.4.0` |
| Go common/env bindings inherited from #10 | `v0.3.0` |
| Published common/env extension definitions | `/extensions/rc/.../v1alpha2/runtimeconditions.extension.yaml` |
| Extension resolver module | `v0.0.0-20261005083147-3cc942274975` |
| Handoff CI/build toolchain | Go `1.27.4` |

The [official CLI release](https://github.com/runtimeconditions/rc-cli/releases/tag/v0.6.0)
is installed by `portable-profile/scripts/install-rc.sh`. Its four archive
checksums match the release's `SHA256SUMS`; cached archives are verified too.
`RC_CLI` selects an explicit executable. Generation also retains upstream's
`RC` override. The handoff runner installs and exports the pinned executable
when neither override is provided. No local source-built profiler is used.

## Catalog and evidence boundary

The generated Profile uses short references such as
`https://runtimeconditions.io/extensions/rc/common-integrations:v1alpha2`.
The producer expands these for retrieval and evidence identity only. The
original serialized Profile, including its generated header, is unchanged.
Short/full aliases of the same extension are rejected as duplicates.

The pinned resolver walks dependency closure and detects cycles and identity
mismatches. The producer captures each URL once, rejects non-HTTPS retrieval,
non-200 responses, oversized or malformed definitions, ambiguous kinds and
merge conflicts, then stores the exact response bytes in a private snapshot.
Explicit `-catalog DIR` selects offline mode; network failures never trigger
fallback to fixtures or alternate versions.

`rc profile validate - --catalog <snapshot>` validates the unmodified Profile
against those same snapshot bytes without another catalog fetch. Signing occurs
only after successful validation. Evidence binds the Profile SHA-256, canonical
extension IDs and byte digests, CLI identity and executable SHA-256, producer
executable SHA-256, and completed validation steps. Ed25519 signs the exact JSON
evidence bytes. Complete bundles are atomically published; signing keys stay
outside received bundles and CI uploads.

The consumer verifies its configured signer, byte identities, and envelope
before calling the existing consumer's support evaluation. It neither resolves
extensions nor links the profiler, resolver, or schema validator. This remains
an experimental evidence format, not an RC schema change, execution attestation,
or production signing infrastructure.

## Previous blockers

The earlier standalone profiler catalog-identity mismatch is avoided by the
released CLI's independent `profile validate` implementation. The old November
19 catalog 404 concerned obsolete `/extensions/.../v1alpha1` identities. The
published `/extensions/rc/.../v1alpha2` identities inherited from #10 now resolve
successfully. No IDs inside published definitions are rewritten to obtain a
passing result.

The fixtures in `internal/producer/testdata` are fresh, unmodified HTTPS responses
captured on November 21, 2026. Offline fixture acceptance remains a separate check
from live published-catalog acceptance.

## Verification on November 21, 2026

- Committed Profile drift check passes without changing the upstream artifact.
- All five required handoff cases pass against live HTTPS publication and,
  separately, against the captured offline fixtures: valid handoff, changed
  Profile, changed extension, untrusted signer, and valid-but-unsupported
  memcached demand.
- Catalog failure/closure tests, canonical-reference and optional-name tests,
  invalid-demand/no-signature checks, and consumer dependency-boundary checks pass.
- Workload and development consumer Go tests and all three modules' `go vet` pass.
- Four Python platform-adapter tests and 23 existing Kratix resolver tests pass.
- The live handoff runner produces `supported` and `unsupported` outcomes with
  both `handoff_verified` and `support_evaluated` true.
- The development demo reaches both todos API and Redis (`ok` for each).
- Java package build (Maven 3.9.11) and Python app installation/published-binding
  imports pass.
- Shell syntax checks, workflow YAML parsing, and `git diff --check` pass.

The local Docker daemon is not running, so local KinD/Kratix cluster acceptance
is unavailable. The separate real-cluster CI job remains enabled and depends on
live handoff acceptance; process-level results do not substitute for that job.

## Reproduce

```sh
export RC_CLI="$(sh portable-profile/scripts/install-rc.sh)"
portable-profile/scripts/generate-profile.sh --check
(cd validated-profile-handoff && go test -v ./...)
(cd validated-profile-handoff && EXTENSIONS_ROOT=internal/producer/testdata go test -v ./...)
python3 -m unittest discover -s validated-profile-handoff/kratix -p 'test_*.py' -v
validated-profile-handoff/scripts/run.sh
portable-profile/scripts/run-dev.sh
# Requires Docker, KinD, kubectl and PyYAML:
validated-profile-handoff/scripts/kind-ci.sh
```
