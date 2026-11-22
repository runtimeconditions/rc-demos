# Offline test fixtures for published extensions

`common.yaml` and `env.yaml` are unmodified HTTPS response bodies captured
on 2026-11-21 from these **versioned, published** definitions:

- https://runtimeconditions.io/extensions/rc/common-integrations/v1alpha2/runtimeconditions.extension.yaml
- https://runtimeconditions.io/extensions/rc/env-configuration/v1alpha2/runtimeconditions.extension.yaml

These files retain the publisher's metadata, dependency IDs, and serialization
byte-for-byte. They exist solely for reproducible, network-independent tests:
dependency closure, retrieval errors, exact-byte identity, and explicitly
selected offline acceptance.

**They are not a private catalog or replacements for published extensions.**
The normal producer retrieves versioned HTTPS definitions and records the
exact bytes it validates using the released CLI. It does not fall back to
these fixtures if retrieval or validation fails. Tests use them only through
local test servers or explicit `-catalog` / `EXTENSIONS_ROOT` mode.

See [the published-catalog guide](../../../PUBLISHED-MIGRATION.md).
