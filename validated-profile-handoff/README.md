# Validated Profile handoff

**Same portable Profile + trusted upstream validation evidence + independent
consumer support evaluation.**

This experiment follows [Portable Profile](../portable-profile/README.md) and
its question about confirming that a received Profile was validated. It tests
one architectural claim:

> A downstream consumer can establish that the exact Runtime Conditions Profile
> it received was resolved and validated upstream, without becoming a Runtime
> Conditions validator itself.

The JSON evidence, detached signature, field names, directory layout, and result
statuses here are deliberately experimental. This is implementation evidence,
not a proposal for a standardized validation-record format or a new Runtime
Conditions resource type. The Profile schema is unchanged.

The primary platform demonstration is now the
[Kratix workflow](kratix/README.md): verification runs as the first Promise
workflow init container, before the existing deployment resolver. A separate
KinD CI job submits all five cases and checks real Pod execution and emitted
Work. The local development runner below remains a fast test harness.

```text
request-logger source declarations
        |
        v  existing go-rc-profiler generation (including its validation)
artifacts/request-logger-http.profile.yaml
        |
        v  cmd/validate: snapshot bytes, resolve closure, validate final artifact
profile.yaml + extensions/*.yaml + evidence.json + evidence.sig
        |
        v  cmd/consume: configured key, signature, exact-byte digests
verified Profile bytes
        |
        v  unchanged portable-profile/cmd/dev-bind: support evaluation
supported demand -> environment bindings written
unsupported demand -> explicit unsupported result, no bindings written
```

Final-artifact validation is intentionally upstream even though generation also
validates. It binds the evidence to the serialized bytes being handed off,
including the generated header. Neither `cmd/consume` nor `dev-bind` repeats RC
semantic validation. The consumer executable has no dependency on the profiler,
its extension checker, or its JSON Schema library; a test checks that boundary.

## Artifacts and upstream resolution

The producer takes the existing generated request-logger Profile unchanged.
For the unsupported case, it separately validates a copy whose cache demand is
`memcached`. That engine is valid in the common-integrations extension, but this
development consumer only supplies Redis. This is a demand variant, not another
workload or a claim that the application's Redis client implements memcached.

| Artifact | Purpose |
| --- | --- |
| `profile.yaml` | Exact canonical portable semantic artifact; no validation fields added |
| `extensions/<sha256>.yaml` | Exact extension definition bytes used by the validator, including transitive dependencies |
| `evidence.json` | Profile SHA-256, resolved extension IDs and SHA-256s, validator identity, producer executable SHA-256, successful upstream API calls |
| `evidence.sig` | Raw Ed25519 signature over the exact `evidence.json` bytes |
| Configured public key | Consumer-side trust policy, provisioned separately from received bundles |
| `request-logger.env` | Consumer-produced development configuration, not part of RC validation evidence |

The current upstream `extensioncheck.ResolveExtensionClosure` and
`extensioncheck.ValidateProfileYAML` APIs resolve `metadata.id` through local
catalog directories. IDs are not fetched as URLs. The resolver checks extension
definitions and dependencies; Profile validation checks the declared closure,
resolved vocabulary, and applicable extension schemas.

The producer copies catalog definition bytes to a private directory before
calling those APIs. It rejects duplicate IDs. It then validates the final
Profile against a second private directory containing only the resolved closure.
Only after both calls succeed does it sign. Thus the evidence covers the bytes
actually available to final validation, rather than hashing mutable source files
after validation. The current request-logger closure contains common-integrations
and env-configuration; the latter depends on the former.

The verifier reads the Profile once, checks a small usable envelope, verifies the
signature, compares its byte digest, compares the Profile's extension IDs with
the signed IDs, and hashes the supplied extension blobs. It does not parse
extension contents, resolve dependency graphs, or evaluate schemas. It passes the
already-verified Profile bytes through a private temporary file to the existing
consumer, avoiding a second read of the received Profile path.

These are byte digests: a comment, newline, or serialization-only change requires
new evidence even if the Profile's meaning is unchanged. The digest filenames
are storage choices, not extension identities. Extra unreferenced files in a
received directory are ignored and never supplied to the consumer.

## Trust assumptions

This experiment uses one explicitly configured Ed25519 public key. The local
runner generates a fresh key pair, makes the private key available only to the
upstream producer invocation, and supplies the public key separately to the
consumer. It removes the private key on exit. CI uses the same local ceremony;
no repository signing secret is needed.

The consumer trusts whoever holds that private key to run the intended producer
and validator faithfully. A signature proves that key authorized these bytes;
it does not independently prove code execution or the validator's correctness.
The signed producer executable digest is an inspectable identity, not remote
attestation. Someone holding the trusted private key can forge a claim. That is
an explicit limit, not something a `validated: true` field would solve.

The demo also trusts its local checkout, compiler, filesystem/process isolation,
and the configured `dev-bind` executable. It assumes no hostile writer inside
the private snapshot directories. This is not isolation from a malicious process
running as the same user.

Trust policy lives with the consumer/operator, outside the received bundle. The
verifier never accepts an embedded public key as its trust anchor. A public key
included in CI artifacts is diagnostic; do not establish trust by accepting a
key from the same untrusted download as its evidence. An actual deployment would
need an independently provisioned trusted key and controlled producer access.

No freshness, expiry, revocation, anti-replay policy, workload-image binding,
source-to-binary provenance, or generally trusted extension publisher is proved.
The current extensions have inline schemas and local fragment references. This
demo does not establish coverage for external schema resources or arbitrary
future resolver inputs. Validation strength remains that of the selected
upstream implementation and extensions.

## Run it

Use Go 1.25 or newer. Clone `extensions` and `go-rc-profiler` beside `rc-demos`, as
for the portable-profile demo. The new CI workflow pins the inspected upstream
revisions so its results can be reproduced:

- `go-rc-profiler`: `5d2e860e48842b89ec83414123a7e59228f61d36`
- `extensions`: `1f24b8815812f8a883cf7ceb87b6a9810d47e106`

From the repository root:

```sh
# Check generation, build both sides, provision local trust, exercise supported
# and valid-but-unsupported handoffs, and retain inspectable public artifacts.
validated-profile-handoff/scripts/run.sh

# Includes all required acceptance cases using real upstream validation and
# the existing dev-bind executable, plus distinct failure-boundary tests.
(cd validated-profile-handoff && go test -v ./...)
```

The runner prints its new `validated-profile-handoff/generated/run.*` directory.
Inspect `valid/evidence.json`, `valid/profile.yaml`, the extension blobs,
`valid-result.json`, and `unsupported-result.json`. Generated output is ignored
by Git. Each run creates new output; the producer refuses an existing output
path, so a failed attempt cannot masquerade as a newly produced bundle.

To exercise each stage manually, from `validated-profile-handoff/`:

```sh
mkdir -p generated/manual
(cd ../portable-profile && go build -o ../validated-profile-handoff/generated/manual/dev-bind ./cmd/dev-bind)
go build -o generated/manual/validate ./cmd/validate
go build -o generated/manual/consume ./cmd/consume

# Use a new private directory outside the received artifacts. Protect this key;
# these commands intentionally leave lifecycle management to the operator.
trust=$(mktemp -d)
go run ./cmd/keygen -out "$trust/keys"
generated/manual/validate \
  -profile ../artifacts/request-logger-http.profile.yaml \
  -catalog ../../extensions/catalog \
  -key "$trust/keys/signer.key" -out generated/manual/bundle
generated/manual/consume \
  -bundle generated/manual/bundle -trusted-key "$trust/keys/trusted.pub" \
  -dev-bind "$PWD/generated/manual/dev-bind" -env-out generated/manual/request-logger.env
rm -rf "$trust"
```

The module uses a sibling `go-rc-profiler` checkout, matching the earlier
composition demo. `EXTENSIONS_ROOT` can override the local catalog path for the
runner and tests. It does not change the existing generator's package discovery.

## Acceptance cases and observed results

The automated tests run the real upstream APIs before producing evidence. They
do not stub validation or declare an arbitrary unsupported Condition valid.

| Case | Handoff | Consumer support | Observed result |
| --- | --- | --- | --- |
| Canonical generated Profile, trusted evidence, original extensions | Pass | Evaluated | `supported`; expected todos API and Redis environment bindings written |
| Profile modified after validation, including only a comment | Fail | Not reached | `profile_digest_mismatch` |
| Different extension bytes under the evidenced digest filename | Fail | Not reached | `extension_mismatch` |
| Structurally valid evidence signed by a different key | Fail | Not reached | `evidence_untrusted` |
| Upstream-validated memcached demand with trusted evidence | Pass | Evaluated | `unsupported`; no environment file written |

Rejected-handoff tests use a sentinel executable to assert that no consumer
process starts. The unsupported case runs the actual development consumer. A
negative producer test also confirms that an invalid engine is rejected upstream
without returning signed artifacts.

The command-line runner produced `handoff_verified: true` and
`support_evaluated: true` for both the canonical and memcached Profiles, with
`supported` and `unsupported` respectively. This is the distinction under test:

```text
invalid/untrusted handoff != valid Profile with unsupported demand
```

Other failure boundaries remain visible:

| Status | Boundary / meaning |
| --- | --- |
| `profile_unusable` | Profile cannot be read as the defensive envelope; also preserves the consumer's unreadable-Profile exit |
| `evidence_missing_or_malformed` | Missing/malformed evidence or signature; unrecognized experiment shape or incomplete calls |
| `profile_digest_mismatch` | Trusted evidence covers different Profile bytes |
| `extension_mismatch` | Extension IDs or supplied extension contents differ from trusted evidence |
| `evidence_untrusted` | Signature does not verify under configured trust policy |
| `unsupported` | Handoff passed; the consumer cannot support the demand |
| `fulfillment_failed` | Handoff and support passed; writing the environment file failed |
| `consumer_execution_failed` | Handoff passed but the consumer could not run normally |
| `trust_configuration_error` | CLI could not load its configured public key |
| `upstream_validation_failed` | Producer did not complete and publish a new handoff bundle |

Checks are ordered; when multiple defects exist, the first rejecting boundary
is reported. Results include booleans for handoff verification and support
evaluation. The CLI exits 0 for supported demand, 3 for unsupported demand, and
1 for failures (2 for missing command-line arguments).

Fulfillment here is deliberately limited to the existing consumer's binding
file write. The failure test passes a directory as the output filename: support
succeeds before the write fails. Without `-env-out`, the command only evaluates
support. Starting services, checking their health, or deploying Kubernetes
resources is outside this wrapper. The existing portable-profile workflow is
unchanged and continues testing the live development demo and Kratix resolver.

## What this demonstrates, and questions it exposes

- **What crosses the boundary?** In this implementation, unchanged Profile bytes,
  opaque extension snapshots, signed evidence, and a detached signature. Trust
  policy arrives separately. That is one executable packaging choice.
- **Is the Profile still canonical?** Yes here: the consumer receives the same
  semantic artifact. Evidence is a sidecar and cannot change its demands.
- **What evidence binds validation?** Exact-byte Profile and closure digests,
  authenticated by a trusted signer, plus the identity and successful calls of
  the upstream validator. The experiment demonstrates that this is sufficient
  under its trust assumptions, not that every included field is necessary.
- **Does a consumer need extension contents?** The existing consumer needs none.
  The verifier receives them here solely to exercise content-mismatch detection.
  It could in principle accept signed digests without blobs when no contents are
  supplied, but that variant has not been implemented or compared. Which inputs
  should a real consumer retain for audit or later interpretation remains open.
- **Where does trust policy live?** In the configured consumer-side public key
  and this wrapper's accepted experiment format/calls. Who provisions that key,
  which validator versions are acceptable, and how policy changes remain open.
- **Can verification avoid RC validation?** Yes for this path: signatures,
  byte identity, and envelope usability are enough before ordinary support
  evaluation. Dependency tests and the valid-but-unsupported case exercise it.
- **Is a new RC concept required?** This implementation needs no new schema,
  normative resource, or spec change. It works as an external pipeline/artifact
  concern. That does not settle whether interoperable evidence is useful later.

This does not compare trust mechanisms, establish the smallest possible evidence
format, standardize validation levels, or prove all consumers need this boundary.
The recorded API calls describe this implementation, not normative RC levels.
The verifier is now exercised by both the local development wrapper and an
opt-in Kratix Promise; neither integration changes the RC Profile schema.
Extension-resolution reporting could eventually remove the need for
the producer's local snapshot/index code; whether that belongs upstream is an
open implementation question.
