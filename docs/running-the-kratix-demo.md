# Running the Kratix demo

From the repository root:

```sh
kratix/scripts/00-check-prereqs.sh
kratix/scripts/01-install-kratix.sh
kratix/scripts/02-install-promises.sh
kratix/scripts/03-deploy-catalog-and-provider.sh
kratix/scripts/04-deploy-application-release.sh
kratix/scripts/05-smoke-test.sh
```

`04-deploy-application-release.sh` embeds the generated
`artifacts/request-logger-http.profile.yaml` into the `ApplicationRelease`
request; the manifest does not carry its own Profile.

To run the contract failure path:

```sh
kratix/scripts/06-demo-breaking-change.sh
```

## Demo images

The image names used by the manifests are intended to live under
`ghcr.io/runtimeconditions/` once publishing workflows are split out:

- `redis-pipeline`
- `application-release-pipeline`
- `todos-api`
- `request-logger`

Until the `ghcr.io/runtimeconditions/*` images are published, a KinD cluster
can use images built from this checkout: run `kratix/scripts/load-local-images.sh`
and then the scripts above with `IMAGE_PULL_POLICY=IfNotPresent`.
