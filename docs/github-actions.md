# Deploying from GitHub Actions

`.github/actions/deploy` in this repo is a composite GitHub Action that
deploys an already-built image to a Levelrail app from your own CI
workflow and, by default, waits for the rollout to actually converge
before the job finishes. It's a thin wrapper over two CLI commands
(`apps deploy`, `apps wait`), not a separate implementation: everything
it does, you could also do by scripting those two commands directly in
a plain shell step.

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

This action never builds your image: build and push it in an earlier
step with whatever your project already uses, then point this action
at the pushed reference. It installs a prebuilt `levelrail-cli` binary
from this repo's own GitHub Releases (`levelrail-cli-<os>-<arch>`,
published for linux and darwin, amd64 and arm64, by
`.github/workflows/release.yml`'s `build-cli-binaries` job) rather than
requiring a Go toolchain in your workflow.

## Not tied to GitHub Actions

Nothing here is GitHub-Actions-specific under the hood: `apps deploy`
and `apps wait` are ordinary `levelrail-cli` commands, real exit codes
included (`apps wait` exits 0 on a converged success, a distinct
non-zero code on a converged failure, and another on timeout, see
`levelrail-cli apps wait -h`). The same two commands work as a
deploy-then-verify pair in GitLab CI, CircleCI, a Jenkinsfile, or a
plain shell script; this Action is just the packaged, no-toolchain-
required version of that pattern for the GitHub Actions ecosystem
specifically.

## Getting the API token and URL into GitHub

- `api-url`: your control plane's own reachable base URL (a domain
  fronted by the embedded Caddy ingress, or `https://host:8080` if
  you haven't set one up yet).
- `api-token`: create one scoped to at least the `deploy` ability
  (Settings -> Tokens, or `levelrail-cli tokens create --abilities deploy`),
  then store it as a repository or environment secret
  (`LEVELRAIL_API_TOKEN` in the example above) so it never appears in
  workflow logs or the workflow file itself.
