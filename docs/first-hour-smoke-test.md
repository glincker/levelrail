---
title: First-hour smoke test
description: "Run the install-to-rollback journey in a Docker container to prove the first hour works, and wire it into a nightly job."
"og:url": "https://docs.levelrail.com/first-hour-smoke-test"
---

`scripts/test-first-hour.sh` replays what a new user does in their first hour, in a systemd Ubuntu 24.04 container on the local Docker: install, setup link, admin, CLI sign-in, deploy a sample app, open it on its zero-config hostname, a second deploy under a request loop, rollback, `doctor`, `domains diagnose` on a made-up hostname, and uninstall. Each step prints a PASS or FAIL line with timings, and the exact output when it fails. It needs no registry login and no GitHub release: the binaries are built locally (or passed in) and the sample app is built on the host and loaded into the container.

```bash
scripts/test-first-hour.sh                 # build, run all 10 steps, clean up
scripts/test-first-hour.sh --keep          # leave the container for debugging
scripts/test-first-hour.sh --keep --steps 1-5
scripts/test-first-hour.sh --container NAME --steps 6-7   # reuse a kept container
scripts/test-first-hour.sh --binary ./levelrail-linux-amd64 --cli ./levelrail-cli-linux-amd64
```

Steps 2 to 10 depend on earlier state, so a `--steps` subset runs against a container kept by a previous `--keep` run. Timeouts are env vars with defaults: `FIRST_HOUR_TIMEOUT` (whole run, 1500s), `FIRST_HOUR_INSTALL_WAIT`, `FIRST_HOUR_READY_WAIT`, `FIRST_HOUR_CUTOVER_WAIT`, `FIRST_HOUR_DOCTOR_WAIT`, `FIRST_HOUR_DIAGNOSE_WAIT`. A new step is one function plus one line in `STEP_TABLE`. It needs a Docker daemon that allows `--privileged` and outbound access for the installer's Docker install, and refuses to start under 5G free.

Not covered: real ACME certificates, a git build, a second node, and a real public IP. The ingress is checked with `curl --resolve`, and the dashboard page is only checked when the binary embeds the web build.

Not wired into a workflow yet. A ready-to-use nightly job, to paste into `nightly.yml`:

```yaml
  # first-hour:
  #   name: first-hour smoke test
  #   needs: check-activity
  #   runs-on: ${{ vars.CI_RUNNER || 'ubuntu-latest' }}
  #   timeout-minutes: 30
  #   steps:
  #     - name: Checkout
  #       uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
  #     - name: Set up Go
  #       uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
  #       with:
  #         go-version-file: go.mod
  #     - name: shellcheck
  #       run: shellcheck -s bash scripts/test-first-hour.sh
  #     - name: First-hour smoke test
  #       run: scripts/test-first-hour.sh
```
