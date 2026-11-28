# The request logger apps

What these are: the same workload in Go, Java and Python. Each copy declares the same
two Conditions (the Todos API and a Redis cache) with the published binding packages,
so no sibling `extensions` checkout is needed. Three language copies prove the bindings
work the same everywhere and give [generating a profile](generating-profiles.md) three
profiling inputs that must agree:

- Go: `runtimeconditions.io/x/rc/common` and `/env` (see `apps/request-logger-http/go.mod`)
- Java: `io.runtimeconditions.bindings` on Maven Central (see the pom)
- Python: `runtimeconditions-common-integrations` and `runtimeconditions-env-configuration` on PyPI

Build each one on its own (from the repository root):

```sh
(cd apps/request-logger-http && go mod download && go vet ./...)
(cd apps/request-logger-http-java && mvn -q -B package)
(cd apps/request-logger-http-python && pip install .)
```

See the [repo README](../README.md) for the layout and quickstart, and
[generating a profile](generating-profiles.md) for turning these declarations
into a committed Profile.
