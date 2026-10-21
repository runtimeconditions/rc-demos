# Published distribution migration: blocked at strict validation

This branch follows [merged PR #5](https://github.com/runtimeconditions/rc-demos/pull/5)
and [Colin's follow-up](https://github.com/runtimeconditions/rc-demos/pull/5#pullrequestreview-5595539492)
about released Go tooling and catalog publication. It is a
**draft**, not evidence that the published-path handoff acceptance contract passes.
No upstream repositories are modified and no validation check is bypassed.

## Compatibility finding

The official profiler v0.6.0 binary successfully generates the committed Profile
from `runtimeconditions.io/x/rc/common@v0.1.0` and `runtimeconditions.io/x/rc/env@v0.1.0`.
Its embedded core schema is 0.4.0. The HTTP contract, response schema, Redis engine,
and the two environment alternatives remain the same. Serialization order changes,
extensions become complete definition URLs, and optional Condition names disappear.
Consumers accept missing names with positional diagnostic labels; explicit invalid
names remain errors. Generated output is not rewritten to add names or change IDs.

The genuine HTTPS catalog returns definitions whose `metadata.id` is that same
complete URL and whose `metadata.version` is the `v1alpha1` annotation. Their
dependencies use the complete URL directly. The current `EXTENSION_RELEASES.md`
also describes an older `<uri>:<version>` publication convention; the actual
published binding resources and HTTPS responses agree on the complete URL IDs.
This demo uses those actual identifiers, without guessing alternate routes.

The released profiler's **catalog** API instead indexes definitions by
`ExtensionReference{ID: metadata.id, Version: metadata.version}`. It parses a
complete Profile/dependency URL as `ExtensionReference{ID: url, Version: ""}`.
Therefore both `ResolveExtensionClosure` and `ValidateProfileYAML` fail to find
the published definition, despite successful HTTP retrieval. This differs from
the installed-generated-binding path used by the executable.

The upstream generic resolver can retrieve and traverse these definitions, but
its merge/Condition validator is not an equivalent replacement for the profiler's
strict vocabulary and definition validation: it intentionally permits ambiguous
kind fallback and has a different validation surface. Substituting it alone would
weaken the original acceptance contract. The producer therefore retains the two
mandatory profiler checks and refuses to sign.

Stripping `metadata.version`, appending versions to generated IDs, validating
rewritten definitions, or signing the original bytes after validating transformed
ones would conceal the mismatch or invalidate the exact-snapshot claim. None of
those workarounds is used here.

## Reproduce without sibling checkouts

```sh
portable-profile/scripts/generate-profile.sh --check
(cd validated-profile-handoff && go test -v ./internal/producer)
(cd validated-profile-handoff && go test -v ./...)
validated-profile-handoff/scripts/run.sh
```

The producer unit tests pass, including a deterministic reproducer using exact
captured publication bytes. The complete acceptance suite and runner fail at
upstream resolution with `missing extension definition ...yaml@` (empty version).
All five end-to-end cases are still enabled; they cannot reach consumer assertions
because their real producer setup fails. Python platform adapter preparation has
the same blocker. This is not a successful five-case acceptance run.

## Required next step

A corrected upstream profiler release needs catalog identity handling consistent
with its generated-binding path, treating `metadata.version` as an annotation for
complete definition URI identities. After such a release is available, update the
module and CLI archive pins, remove/update the incompatibility reproducer, and
rerun every acceptance case and the genuine KinD/Kratix job. No upstream change
is made as part of this rc-demos task.

The generic resolver is pinned at
`v0.0.0-20261005083147-3cc942274975` because it has no published version tags.
It requires Go 1.27, so CI uses Go 1.27.3. Its graph traversal is used upstream
only. The downstream verifier does not import either resolver or validator.

## Validation evidence and limitations

The original evidence format and signing flow are unchanged. Evidence covers the
exact Profile bytes (including the generated header) and SHA-256 digests of every
resolved extension response; the signature covers the exact JSON evidence bytes.
Complete bundles are atomically renamed into place only after strict validation;
keys remain outside published output. Unavailable resources, invalid definitions,
conflicts, and validation errors produce no signed artifacts and no fallback.

This remains experimental implementation evidence, not a proposed standardized
evidence format or production-grade signing infrastructure. The original trust,
replay, publisher, and deployment limitations remain.

## Local verification (2026-10-21)

- Workload Go tests, portable consumer Go tests, and Profile drift check pass.
- Producer catalog tests pass (HTTP fixture failures, closure, exact bytes,
  conflicts/cycles, response limits, explicit HTTPS policy, and the incompatibility
  reproducer). The consumer dependency boundary test passes.
- Existing Python ApplicationRelease resolver suite: 24 tests pass, including
  canonical Redis/Deployment/Service output and optional-name handling.
- Go vet passes for the workload, portable consumer, and handoff module.
- Five changed/new shell scripts pass syntax checks; both workflow YAML files
  parse and retain `contents: read`. ShellCheck is not installed locally.
- Full handoff Go acceptance suite, Python platform adapter setup, and the real
  runner fail at the released catalog identity mismatch described above.
- Docker, KinD, and kubectl are unavailable locally. The cluster job is retained
  in CI and depends on the handoff job; it cannot run successfully until the
  producer blocker is fixed. No cluster acceptance pass is claimed.
- The live development demo also needs Redis or Docker, unavailable locally;
  its existing CI step is retained.
