# Runtime Conditions Demos

`rc-demos` is the hands-on playground for
[Runtime Conditions](https://github.com/runtimeconditions): runnable workload apps,
committed [Runtime Conditions Profiles](https://github.com/runtimeconditions/spec)
generated from them, and downstream adapter demos that fulfill those profiles with real
platforms: a local development binder, Kratix, and a KinD + Cilium + Backstage lab.

A Runtime Conditions Profile is a portable, machine-readable list of what one workload
needs at runtime: the APIs, caches, datastores, and environment configuration it depends
on. The demos here show the whole loop. The workload declares the need once in its source,
the [`rc-cli`](https://github.com/runtimeconditions/rc-cli) generates the Profile,
validation checks it against the spec, and a platform fulfills it.

**Example scenario:** everything in this repo revolves around one workload, the
[request logger](apps/request-logger-http). It depends on a [Todos API](apps/todos-api)
reachable at `TODOS_API_URL` and a Redis cache at `REDIS_URL`. Its committed Profile,
[`artifacts/request-logger-http.profile.yaml`](artifacts/request-logger-http.profile.yaml),
is exactly what the demos in this repo fulfill, in a local dev environment and on Kratix.

- Guide and concepts: <https://runtimeconditions.io>
- Profile specification: <https://github.com/runtimeconditions/spec> defines the profile
  format every demo profile follows
- Tooling: [`rc-cli`](https://github.com/runtimeconditions/rc-cli) generates and validates
  the Profiles committed under `artifacts/`

## Quickstart

Check the committed demo Profile against the core schema embedded in `rc` and the
definitions of the extensions it declares:

```console
$ git clone https://github.com/runtimeconditions/rc-demos.git
$ rc profile validate rc-demos/artifacts/request-logger-http.profile.yaml
runtimeconditions: profile validated
```

Install `rc` first if you don't have it. Either use Homebrew, or configure the apt / dnf
repositories that setup.sh provides:

```sh
brew install runtimeconditions/tools/rc-cli
curl -fsSL https://runtimeconditions.io/packages/setup.sh | sudo sh
sudo apt install rc-cli          # or: sudo dnf install rc-cli
```

Then pick a demo to run end to end: [Running the Kratix demo](docs/running-the-kratix-demo.md)
spins up a full KinD cluster with Kratix, while the [other guides](#documentation) cover the
lighter-weight experiments.

## What's here

| Path | What it is |
| --- | --- |
| [`docs/`](docs/) | How-to guides for this repo: the request logger apps, generating profiles, running the Kratix demo |
| [`apps/request-logger-http/`](apps/request-logger-http/) | Go workload that imports first-party declaration packages and demonstrates explicit profile declarations |
| [`apps/request-logger-http-java/`](apps/request-logger-http-java/) | Java workload with matching explicit declarations for the same Conditions as the Go request logger |
| [`apps/request-logger-http-python/`](apps/request-logger-http-python/) | Python workload declaring the same Conditions via the PyPI bindings |
| [`apps/todos-api/`](apps/todos-api/) | Simple provider API used by the request logger demo |
| [`artifacts/`](artifacts/) | The generated, committed request-logger Profile ([`request-logger-http.profile.yaml`](artifacts/request-logger-http.profile.yaml)) shared by the consumers |
| [`catalog/apis/`](catalog/apis/) | OpenAPI and catalog files for the Todos API, used by the adapter demos |
| [`portable-profile/`](portable-profile/) | One generated request-logger Profile consumed by both a local development environment and the Kratix deployment path |
| [`kratix/`](kratix/) | Kratix Promises, manifests, and scripts that fulfill the Profile on a cluster |
| [`validated-profile-handoff/`](validated-profile-handoff/) | Verifying trusted upstream validation evidence for exact Profile and extension artifacts before consumer support evaluation |
| [`dev-container-profile/`](dev-container-profile/) | Development scaffold and acceptance contract for a build-time Profile composition demo |
| [`cilium-policy/`](cilium-policy/) | A complete KinD, Cilium, Backstage, and microservice lab for building an RCProfile-to-CiliumNetworkPolicy adapter |

## Documentation

Guides for this repo:

- [The request logger apps](docs/request-logger-apps.md): Go, Java and Python bindings and build commands
- [Generating a profile from an app](docs/generating-profiles.md): profilers and `rc profile generate` per language
- [Running the Kratix demo](docs/running-the-kratix-demo.md): cluster setup, deployment, and the breaking-change path

Each demo has its own guide:

- [Portable Profile](portable-profile/README.md): one demand, two fulfillments
  (local dev binder and Kratix)
- [Validated Profile handoff](validated-profile-handoff/README.md): trusted upstream
  validation evidence for a received Profile; see also
  [PUBLISHED-MIGRATION.md](validated-profile-handoff/PUBLISHED-MIGRATION.md) for pinned
  dependencies and reproduction commands
- [Dev-container Profile](dev-container-profile/README.md): build-time Profile
  composition; start from [PROJECT-BRIEF.md](dev-container-profile/PROJECT-BRIEF.md),
  design notes in [COMPOSITION-NOTES.md](dev-container-profile/COMPOSITION-NOTES.md)
- [Cilium policy lab](cilium-policy/README.md): RCProfile-to-CiliumNetworkPolicy
  adapter; contract in [docs/adapter-contract.md](cilium-policy/docs/adapter-contract.md),
  setup in [docs/manual-install.md](cilium-policy/docs/manual-install.md)

## Ecosystem

The full list of Runtime Conditions repositories lives on the
[organization page](https://github.com/runtimeconditions) and in
[rc-cli's README](https://github.com/runtimeconditions/rc-cli#ecosystem). The repos this
demo repo connects to directly:

- [rc-cli](https://github.com/runtimeconditions/rc-cli): the `rc` binary that
  generates (`rc profile generate`) and validates (`rc profile validate`) the
  profiles in `artifacts/`; the [quickstart](#quickstart) validates one of them
- [spec](https://github.com/runtimeconditions/spec): defines the Profile format;
  `rc` pins the core schema at build time and the committed Profiles follow it
- [extensions](https://github.com/runtimeconditions/extensions): the extension
  vocabulary and schemas the Profiles declare; the apps import the published binding
  packages built from it
- [go-rc-profiler](https://github.com/runtimeconditions/go-rc-profiler): compiled
  into `rc`, profiles the Go app
- [java-rc-profiler](https://github.com/runtimeconditions/java-rc-profiler) and
  [python-rc-profiler](https://github.com/runtimeconditions/python-rc-profiler):
  add-ons that profile the Java and Python request loggers
- [backstage-plugin](https://github.com/runtimeconditions/backstage-plugin): consumes
  profiles in Backstage; exercised by the [cilium-policy](cilium-policy/README.md) lab

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE): Copyright (c) 2026 Runtime Conditions.
