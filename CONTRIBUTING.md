# Contributing

Thanks for your interest in the Runtime Conditions demos.

## Where the demos run in CI

Each demo has its own workflow under [`.github/workflows/`](.github/workflows):

| Workflow | Demo |
| --- | --- |
| `portable-profile.yaml` | `portable-profile/` |
| `validated-profile-handoff.yaml` | `validated-profile-handoff/` |
| `dev-container-profile.yaml` | `dev-container-profile/` |

## Changing a demo

1. Make the change inside the demo directory and update its README.
2. Keep committed artifacts in sync with source: `portable-profile/scripts/generate-profile.sh --check`
   must pass against `artifacts/request-logger-http.profile.yaml`.
3. Update the top-level [README.md](README.md) when the layout, commands, or links change.

## Issues and questions

Open an [issue](https://github.com/runtimeconditions/rc-demos/issues) in this repo for
bugs, broken demos, or requests. For questions about Runtime Conditions itself, start at
<https://runtimeconditions.io>.
