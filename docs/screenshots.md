# Dashboard screenshots

The PNG files in `docs/assets/screenshots/` are captured from a real, running control plane. They show real deployed containers, real metrics, and real log lines, with no mocking or hand-editing.

Screenshots are produced by `scripts/screenshots/capture.sh`, a manual maintainer tool (not a CI job). Deploying real containers and accumulating metrics takes a couple of minutes, too slow for every PR. Re-run it by hand whenever the dashboard changes significantly and existing screenshots look stale.

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

The script performs these steps:

1. Build fresh binaries into a scratch directory
2. Start a control plane against a separate scratch data directory
3. Deploy two real apps from public images
4. Trigger real redeploys to build deploy history
5. Send real HTTP traffic to the apps
6. Wait about 75 seconds for metrics to accumulate at 15-second resolution
7. Log in through the real `/login` form
8. Capture screenshots with `shot-scraper`

It cleans up after itself on exit (including on failure): the deployed
apps are deleted, the control plane process is stopped, and the scratch
directory is removed. It's safe to re-run any time; each run picks a
free local port rather than assuming one is available, and overwrites
the five PNGs in place.

Set `KEEP_SCRATCH=1` to leave the scratch directory (binaries, data dir,
server log, session state) in place after a run, useful for debugging a
failed capture.

## Why not dev-mode's fixed tokens

The control plane binary builds with `-tags embedweb` to serve the real built frontend (same as a user would run). This also compiles out the `APP_DEV_MODE` bypass, so dev-mode's fixed tokens aren't available.

Instead, the script:

1. Bootstraps a real admin account using `APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD`
2. Logs into the browser through the actual login form
3. Authenticates the CLI using the real device-login flow (`levelrail-cli auth login --device`)
4. Approves the login through the browser

All credentials are freshly created for each run and never leave the scratch data directory.

## Files

- `scripts/screenshots/shots.yml`: the `shot-scraper multi` config, one
  entry per PNG.
- `scripts/screenshots/login_state.py`: logs into a running control
  plane through the real login form and saves the session as a
  Playwright storage state file, reused across all five shots.
- `scripts/screenshots/capture.sh`: orchestrates the whole pipeline.
