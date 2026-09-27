# VPS smoke test

`scripts/vps-smoke.sh` checks a real install end to end on a VPS you already have. It never creates or deletes cloud resources.

## What it does

1. Runs this tree's `install.sh` over SSH at `SMOKE_FROM_TAG`.
2. Reads the one-time setup token, creates the admin, and mints an API token.
3. Pushes three fixtures (Next.js, Go API, static, under `test/fixtures/smoke`) to a scratch branch, connects the git source with a GitHub token, and deploys each on `<app>.<SMOKE_DOMAIN_BASE>`.
4. Asserts each domain serves a verified TLS chain with at least 7 days before `notAfter`.
5. Pushes a second commit, redeploys, rolls back to the previous succeeded deploy, and checks the served body marker returns to the first commit.
6. Upgrades to `SMOKE_TO_TAG` while curling every `/healthz` about ten times a second, and fails on any non-2xx.

It prints a summary table and exits non-zero on any failure.

## Prerequisites

- An Ubuntu VPS reachable by SSH as root or a passwordless sudo user, ports 80, 443 and 8080 open.
- A wildcard A record for `SMOKE_DOMAIN_BASE` pointing at it.
- A scratch GitHub repo and a token with contents write on it. The run pushes and deletes a `smoke/<timestamp>` branch.
- On the runner: `ssh`, `curl`, `jq`, `openssl`, `git`, and Go (or `SMOKE_CLI` pointing at a `levelrail-cli` binary).

## Run

```
SMOKE_HOST=root@203.0.113.10 SMOKE_DOMAIN_BASE=smoke.example.com \
SMOKE_REPO=owner/scratch SMOKE_GH_TOKEN=... \
SMOKE_FROM_TAG=v0.1.0-beta.1 SMOKE_TO_TAG=v0.1.0-beta.2 \
scripts/vps-smoke.sh
```

`--dry-run` prints the planned steps without touching the network. Use a fresh VPS for each run, since install and admin creation are one-time.

## CI

The `VPS smoke` workflow is `workflow_dispatch` only. It takes the values above as inputs and needs the `SMOKE_GH_TOKEN` and `SMOKE_SSH_KEY` secrets.
