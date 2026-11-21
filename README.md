# Runtime Conditions Demos

Runtime Conditions is currently seeking adoption by an established parent
project. The repositories in this organization are split for hands-on usability,
review, demos, and implementation feedback. They are not intended to present
Runtime Conditions as a standalone foundation or competing project.

Start here: https://runtimeconditions.github.io/

This tree contains runnable examples and downstream adapter assets.

## Layout

- `apps/request-logger-http/` - Go workload that imports first-party declaration packages and demonstrates explicit profile declarations.
- `apps/request-logger-http-java/` - Java workload with matching explicit declarations for the same Conditions as the Go request logger.
- `apps/todos-api/` - simple provider API used by the request logger demo.
- `portable-profile/` - experiment in which one generated request-logger Profile is consumed by both a local development environment and the Kratix deployment path. Start with [`portable-profile/README.md`](portable-profile/README.md).
- `validated-profile-handoff/` - experiment in verifying trusted upstream validation evidence for exact Profile and extension artifacts before consumer support evaluation. Start with [`validated-profile-handoff/README.md`](validated-profile-handoff/README.md).
- `artifacts/request-logger-http.profile.yaml` - the generated, committed request-logger Profile shared by both consumers.
- `dev-container-profile/` - development scaffold and acceptance contract for a build-time Profile composition demo. Implementers should start with [`dev-container-profile/README.md`](dev-container-profile/README.md).
- `catalog/apis/` - OpenAPI and catalog files used by the adapter demo.
- `kratix/` - Kratix Promise and adapter assets for downstream fulfillment demos.
- `kratix/manifests/` - static Kubernetes and Kratix manifests applied by the demo scripts.
- `cilium-policy/` - a complete KinD, Cilium, Backstage, and microservice lab for
  building an RCProfile-to-CiliumNetworkPolicy adapter.

## The Request Logger apps

The request logger exists in Go, Java and Python. Each declares the same two
Conditions (the todos API and a Redis cache) with the published binding
packages, so no sibling `extensions` checkout is needed:

- Go: `runtimeconditions.io/x/rc/common` and `/env` (see `apps/request-logger-http/go.mod`)
- Java: `io.runtimeconditions.bindings` on Maven Central (see the pom)
- Python: `runtimeconditions-common-integrations` and `runtimeconditions-env-configuration` on PyPI

Build each one on its own:

```sh
(cd apps/request-logger-http && go mod download && go vet ./...)
(cd apps/request-logger-http-java && mvn -q -B package)
(cd apps/request-logger-http-python && pip install .)
```

## Generate a Profile from an app

Install the `rc` CLI, then the profiler for the language:

```sh
# rc CLI: Homebrew, or the apt / dnf repositories that setup.sh configures
brew install runtimeconditions/tools/rc-cli
curl -fsSL https://runtimeconditions.io/packages/setup.sh | sudo sh
sudo apt install rc-cli          # or: sudo dnf install rc-cli

# Java profiler (needs JDK 17+)
brew install runtimeconditions/tools/rc-profiler-java    # or: sudo apt install rc-profiler-java

# Python profiler: it finds the bindings through the environment it is installed in,
# so it has no system package. Install it into the app's virtual environment and
# put that environment's bin directory on PATH.
python3 -m venv .venv && . .venv/bin/activate
pip install runtimeconditions-profiler
```

Then generate:

```sh
# Go (built into rc)
rc profile generate --dir apps/request-logger-http --out request-logger-http.profile.yaml

# Java
rc profile java generate --project apps/request-logger-http-java --resolve-build-classpath \
  --name request-logger-http \
  --workload-uri github.com/runtimeconditions/rc-demos/apps/request-logger-http-java \
  --workload-version dev

# Python
rc profile python generate --project apps/request-logger-http-python \
  --name request-logger-http \
  --workload-uri github.com/runtimeconditions/rc-demos/apps/request-logger-http-python \
  --workload-version dev
```

All three produce the same API and cache Conditions, with one difference: the Go
profile also includes the HTTP response schema of the todos API (completed, id,
title). The Java demo leaves it out because the Java profiler cannot yet read the
enum constants that name the field types, and the Python demo does not declare it
yet, so those two profiles omit it.

**Condition names.** The profilers do not give Conditions a `name` (it is
optional in core 0.4.0). The Kratix resolver and the development binder name
such Conditions after their kind (`api-1`, `cache-1`) in their messages.

`portable-profile/scripts/generate-profile.sh` runs `rc profile generate` on the
Go app (put `rc` on `PATH`, or set `RC=/path/to/rc`) and writes
`artifacts/request-logger-http.profile.yaml`; use `--check` to confirm the
committed Profile still matches source.

## Published Demo Images

The image names used by the manifests are intended to live under
`ghcr.io/runtimeconditions/` once publishing workflows are split out.

- `redis-pipeline`
- `application-release-pipeline`
- `todos-api`
- `request-logger`

## Run the Kratix Demo

From the repository root:

```sh
kratix/scripts/00-check-prereqs.sh
kratix/scripts/01-install-kratix.sh
kratix/scripts/02-install-promises.sh
kratix/scripts/03-deploy-catalog-and-provider.sh
kratix/scripts/04-deploy-application-release.sh
kratix/scripts/05-smoke-test.sh
```

Until the `ghcr.io/runtimeconditions/*` images are published, a KinD cluster
can use images built from this checkout: run `kratix/scripts/load-local-images.sh`
and then the scripts above with `IMAGE_PULL_POLICY=IfNotPresent`.

`04-deploy-application-release.sh` embeds the generated
`artifacts/request-logger-http.profile.yaml` into the `ApplicationRelease`
request; the manifest does not carry its own Profile.

To run the contract failure path:

```sh
kratix/scripts/06-demo-breaking-change.sh
```
