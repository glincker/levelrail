# Dashboard screenshots

`docs/assets/screenshots/*.png` (used in the README and elsewhere in
`/docs`) are captured from a real, running control plane: real deployed
containers, real metrics data points, real log lines. Nothing in them is
mocked or hand-edited.

They're produced by `scripts/screenshots/capture.sh`, a maintainer-run
tool, not a CI job. Deploying real containers and letting metrics
accumulate takes a couple of minutes, which is too slow and too heavy to
run on every PR; re-run it by hand whenever the dashboard changes enough
that the existing screenshots look stale.

## Prerequisites

- Docker, running locally (`docker info` should succeed)
- Go and Node.js/npm (to build the control plane, CLI, and frontend)
- Python 3 with Playwright installed, and
  [shot-scraper](https://shot-scraper.datasette.io/):
  ```
  pip install playwright shot-scraper
  playwright install chromium
  ```

## Running it

```
scripts/screenshots/capture.sh
```

This builds fresh binaries into a scratch directory, starts a control
plane against a scratch data directory (never the repo's own dev data),
deploys two real apps from public images, triggers a couple of real
redeploys on one of them so there's real deploy history, sends it some
real HTTP traffic, waits about 75 seconds for metrics to accumulate at
their normal 15-second resolution, then logs in through the actual
`/login` form and drives `shot-scraper` against the real dashboard.

It cleans up after itself on exit (including on failure): the deployed
apps are deleted, the control plane process is stopped, and the scratch
directory is removed. It's safe to re-run any time; each run picks a
free local port rather than assuming one is available, and overwrites
the five PNGs in place.

Set `KEEP_SCRATCH=1` to leave the scratch directory (binaries, data dir,
server log, session state) in place after a run, useful for debugging a
failed capture.

## Why not dev-mode's fixed tokens

The control plane binary this script builds uses `-tags embedweb` so it
serves the real built frontend, the same binary a user would run. That
build tag also compiles out the `APP_DEV_MODE` bypass entirely (see
`internal/api/devmode_release.go`), so `dev-fixtures.yml`'s fixed API
tokens aren't available here. The script bootstraps a real `dev`/`dev`
admin account instead (`APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD`, which
work regardless of build tags), logs into it for real through the
browser, and authenticates the CLI via the real device-login flow
(`levelrail-cli auth login --device`), approved through the real approval
endpoint using that same browser session. Every credential involved is
freshly minted for the run and only ever touches the scratch data
directory.

## Files

- `scripts/screenshots/shots.yml`: the `shot-scraper multi` config, one
  entry per PNG.
- `scripts/screenshots/login_state.py`: logs into a running control
  plane through the real login form and saves the session as a
  Playwright storage state file, reused across all five shots.
- `scripts/screenshots/capture.sh`: orchestrates the whole pipeline.
