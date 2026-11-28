# Generating a profile from an app

Install the `rc` CLI (see the [quickstart](../README.md#quickstart)), then the
profiler for the language:

```sh
# Java profiler (needs JDK 17+)
brew install runtimeconditions/tools/rc-profiler-java    # or: sudo apt install rc-profiler-java

# Python profiler: it finds the bindings through the environment it is installed in,
# so it has no system package. Install it into the app's virtual environment and
# put that environment's bin directory on PATH.
python3 -m venv .venv && . .venv/bin/activate
pip install runtimeconditions-profiler
```

Go profiling is built into `rc`, so Go workloads need nothing else. Then generate
(from the repository root):

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
