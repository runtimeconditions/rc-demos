# Published catalog fixtures

`common.yaml` and `env.yaml` are exact HTTPS responses captured on 2026-10-21:

- https://runtimeconditions.io/extensions/common-integrations/v1alpha1/runtimeconditions.extension.yaml
- https://runtimeconditions.io/extensions/env-configuration/v1alpha1/runtimeconditions.extension.yaml

They retain the publisher's version annotations and dependency identities. Tests
serve these bytes from local HTTP fixtures to make retrieval errors and the
released catalog API mismatch reproducible. Integration and acceptance tests
continue using the actual published HTTPS path by default.
