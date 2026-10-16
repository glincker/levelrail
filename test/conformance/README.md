# Deploy conformance runner

Drives the real `levelrail-cli` against a control plane: import (detect), deploy,
wait, HTTP probe, redeploy, rollback, delete, for each repo in `fixtures.json`.
The fixtures are private repos `glincker/lr-qa-<name>` with no Dockerfile or
platform config (except the dockerfile and compose ones), so the platform has to
detect framework, port and start command itself.

## Run

```bash
# local control plane (profile "preview" points at localhost:8080)
APP_PROFILE=preview test/conformance/run.sh

# a subset, keep the apps afterwards, 20 minute cap per fixture
APP_PROFILE=preview test/conformance/run.sh --only express,django --keep --timeout 1200

# plumbing check against one image fixture (traefik/whoami), no git access needed
APP_PROFILE=preview test/conformance/run.sh --selftest
```

Flags: `--only a,b`, `--wave N` (fixtures.json `wave` field), `--jobs N` (default 4
concurrent fixtures), `--load` (everything at once), `--resume RUN` (skip passed),
`--failed-from RUN` (rerun only failures), `--report RUN` (rebuild matrix files),
`--run-id RUN`, `--keep`, `--timeout SECONDS` (per fixture, default 900),
`--host-header` (probe the ingress on 127.0.0.1:${INGRESS_PORT:-80} with the app
domain as Host instead of the published host port), `--i-know`, `--selftest`.

Environment: `APP_PROFILE` or `APP_API_URL` plus `APP_API_TOKEN` choose the
control plane. `LEVELRAIL_CLI` uses an existing CLI binary, otherwise
`./cmd/levelrail-cli` is built once into a temp dir. `GIT_TOKEN` is described
below. `INGRESS_PORT` applies to `--host-header`.

Needs `jq`, `curl`, and `go` (unless `LEVELRAIL_CLI` is set).

## Against vps2

```bash
APP_PROFILE=vps2 test/conformance/run.sh --i-know --only express
```

vps2 is a real server with real data. Add its host to `allowed-hosts` only when
you mean it; `--i-know` is the one-off override. The runner never touches apps
whose names do not start with `qa-`, never deletes an app that existed before
the run (it reports `preexisting` and skips it), and only deletes apps it
created itself (`apps delete --delete-data --confirm NAME`, so volumes go too).
Off localhost it probes the app's `fallback_url`, then `http://<host>:<host_port>`.

## Safety

`allowed-hosts` lists API hosts the runner may target (default `localhost` and
`127.0.0.1`). Any other API host exits 3 unless `--i-know` is passed.

## How private repos are resolved

`import <repo-url> --analyze` and `--apply` read the repo through the provider
API, and builds clone it, using one of:

1. The GitHub App connected to the control plane, installed on the `glincker`
   org with access to the `lr-qa-*` repos. This is the normal path; nothing
   else to configure in the runner.
2. A per-app personal access token stored on the app's git source
   (`apps git-source set --token-secret`).

The CLI has no token flag on `import`, so when the GitHub App is not installed
set `GIT_TOKEN` (a PAT with read access to the repos, for example
`GIT_TOKEN=$(gh auth token)`). The runner then analyzes through
`POST /api/v1/imports/analyze` with a one-shot token (never stored), creates the
app with `apps create`, stores the token with `apps git-source set`, and
triggers the build with `apps builds trigger`. The token is never printed.
Public mirrors need neither: point a fixture's `repo` at a public repo.

## Output

Per-fixture `results/<run>/<name>.json` and `.log`, live state in
`results/progress.txt` (queued, building, checking, passed, failed,
unsupported), merged into `results/<run>.json` and `.md` with a load section
(wall time, peak builds in flight, queue wait, build seconds). A failure with
the platform's "supported yet" message is `unsupported`, not a harness failure.
Add `alt_ref` (a branch with a different commit) to a fixture so the rollback
check has a distinct deploy; without it a refused rollback is only a note.

Each run writes `results/<timestamp>.json` (raw rows) and `results/<timestamp>.md`
(a docs-friendly matrix with version, commit and date), and prints a summary
table. Exit code is 1 when any fixture recorded a failure stage. Rows hold:
`detect_ok build_ok run_ok http_ok health_ok redeploy_ok rollback_ok seconds
failure_stage failure_excerpt` plus what was detected. `redeploy_ok` and
`rollback_ok` are `null` (shown `-`) when skipped. Failure stages: `detect`,
`deploy`, `build`, `run`, `http`, `health`, `redeploy`, `rollback`, `timeout`,
`preexisting`. Detection mismatches do not stop the deploy, so later stages are
still recorded.

A detect check passes when the expected framework is a case-insensitive
substring of the detected `framework / runtime`, the port matches, and the
build kind matches (`static` needs build type `static`, `compose` accepts
`compose` or `dockerfile`, `container` rejects `static`). A health path that
differs from the detected one is recorded in the note, not failed.

## Add a fixture

1. Create `glincker/lr-qa-<name>` whose `/` returns 200 with the text
   `lr-qa <name> ok`.
2. Add an entry to `fixtures.json`:

```json
{
  "name": "<name>",
  "repo": "glincker/lr-qa-<name>",
  "expect": { "framework": "Express", "port": 3000, "kind": "container", "health_path": "/" },
  "root_dir": "apps/web",
  "notes": "optional"
}
```

`root_dir` is the monorepo app directory (passed as `--base-directory`).
Several entries may share one repo (see `monorepo-paths-*`); then set
`expect.body_contains` if the text is not derived from the repo name. An entry
with `image` instead of `repo` deploys a prebuilt image (the `selftest` list
uses this).
