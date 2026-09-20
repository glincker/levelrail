# Deploying from GitHub Actions

`.github/actions/deploy` is a composite GitHub Action that deploys an already-built image to a Levelrail app and waits for the rollout to converge before the job finishes.

It's a thin wrapper over two CLI commands: `apps deploy` and `apps wait`. Everything it does, you can script directly in a shell step.

Full inputs/outputs reference and a complete example workflow live in
the action's own [`.github/actions/deploy/README.md`](https://github.com/glincker/levelrail/blob/main/.github/actions/deploy/README.md).
The short version:

```yaml
- name: Deploy to Levelrail
  uses: glincker/levelrail/.github/actions/deploy@main
  with:
    api-url: ${{ secrets.LEVELRAIL_API_URL }}
    api-token: ${{ secrets.LEVELRAIL_API_TOKEN }}
    app: my-app
    image: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

This action does not build your image: build and push it separately using whatever your project already uses, then pass the image reference to this action.

The action installs a prebuilt `levelrail-cli` binary from GitHub Releases (available for linux/darwin, amd64/arm64) instead of requiring a Go toolchain in your workflow.

## Not tied to GitHub Actions

The underlying commands (`apps deploy` and `apps wait`) are ordinary `levelrail-cli` commands with real exit codes. The same pattern works in GitLab CI, CircleCI, Jenkinsfiles, and shell scripts.

This GitHub Action is just the packaged, no-toolchain-required version for GitHub Actions specifically.

## Getting the API token and URL into GitHub

Set two secrets in your GitHub repository or environment:

**`api-url`**: your control plane's base URL. Either a domain fronted by the embedded Caddy ingress or `https://host:8080`.

**`api-token`**: create a token with at least the `deploy` ability using Settings -> Tokens or `levelrail-cli tokens create --abilities deploy`. Store it as a repository or environment secret so it never appears in workflow logs.
