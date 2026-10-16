# Monorepo fixture

A pnpm workspace with two tiny services, `services/api` and `services/web`, each with its own Dockerfile.

Use it to try workspace detection and affected-only deploys:

1. Connect this directory as a git source and run `apps git-source detect <app>`; both packages are listed.
2. Apply one with `--package services/web`. Pushes that only touch `services/api` are then skipped.
3. A change to `pnpm-lock.yaml` or `pnpm-workspace.yaml` redeploys both services.
