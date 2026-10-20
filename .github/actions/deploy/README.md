# Levelrail Deploy Action

A composite GitHub Action that deploys an already-built image to a
Levelrail app and (by default) waits for the rollout to actually
converge, not just for the trigger call to be accepted. Wraps
`levelrail-cli apps deploy` and `apps wait`; installs a matching
prebuilt CLI binary from this repo's own GitHub Releases, no Go
toolchain required in the calling workflow.

This action never builds your image. Build and push it with whatever
your project already uses (`docker/build-push-action`, a Makefile
target, Bazel, anything that ends with a pushed, pullable image
reference), then call this action to tell Levelrail to run it.

## Usage

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - name: Build and push image
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}

      - name: Deploy to Levelrail
        uses: glincker/levelrail/.github/actions/deploy@main
        with:
          api-url: ${{ secrets.LEVELRAIL_API_URL }}
          api-token: ${{ secrets.LEVELRAIL_API_TOKEN }}
          app: my-app
          image: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

Pin to a released tag instead of `@main` once one exists (e.g.
`glincker/levelrail/.github/actions/deploy@v0.3.0`) for a stable,
reviewable version rather than tracking this repo's own default branch.

## Inputs

| Input | Required | Default | Description |
| --- | --- | --- | --- |
| `api-url` | yes | | Levelrail control plane base URL |
| `api-token` | yes | | API token with at least the `deploy` ability |
| `app` | yes | | Name of the existing Levelrail app to deploy |
| `image` | yes | | Image reference to deploy (already pushed) |
| `confirm` | no | `false` | Confirm deploying into a protected environment |
| `wait` | no | `true` | Wait for the rollout to converge before finishing |
| `timeout` | no | `5m` | Max time to wait (Go duration string), only used when `wait` is `true` |
| `cli-version` | no | `latest` | `levelrail-cli` release tag to install |

## Outputs

| Output | Description |
| --- | --- |
| `status` | `succeeded`, `failed`, or `skipped` (when `wait: false`) |

## Why "wait" matters

`POST /api/v1/apps/{name}/deploys` (what `apps deploy` calls) returns
the instant the desired-state write lands, before the application
controller's reconcile loop has run even once. Without waiting, this
step (and the job it's in) reports success the moment the request was
merely *accepted*, not once the new container is actually up and
healthy. `wait: true` (the default) makes this action's own step fail,
with the real reason, if the rollout doesn't converge within `timeout`
(a crash, a failed readiness probe, an OOM kill during startup all
surface here instead of only being visible later in the dashboard).

## Requirements on the target Levelrail instance

- The app named by `app` must already exist (create it once via the
  dashboard, `apps create`, or a separate provisioning step; this
  action only ever redeploys an existing app's image).
- The API token needs the `deploy` ability at minimum.
- The control plane must be reachable from GitHub's own runners (a
  public IP, or a self-hosted runner with network access to it).
