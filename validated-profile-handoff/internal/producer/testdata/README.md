# Published catalog fixtures

`common.yaml` and `env.yaml` are exact HTTPS responses captured on 2026-11-21:

- https://runtimeconditions.io/extensions/rc/common-integrations/v1alpha2/runtimeconditions.extension.yaml
- https://runtimeconditions.io/extensions/rc/env-configuration/v1alpha2/runtimeconditions.extension.yaml

These are the identities emitted by the published v0.3.0 Go bindings used in
merged PR #10. The fixtures preserve the publisher's version annotations,
dependency identities, and serialization without modification. Catalog tests
serve them locally to exercise dependency traversal and retrieval failures;
the released CLI validates the same bytes in offline acceptance. Live acceptance
and Kratix preparation continue using the published HTTPS URLs with no fallback.
