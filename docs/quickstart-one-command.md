---
title: "One command quickstart"
icon: "/assets/icons/rocket-launch.svg"
sidebarTitle: "Quickstart: one command"
description: Deploy a project with `levelrail-cli up`, which infers the name, port and build, then keep every advanced flag when you need it.
"og:url": "https://docs.levelrail.com/quickstart-one-command"
---

If your project is in a git repository with a remote, three commands take it from your laptop to a running app on your server:

```bash
levelrail-cli login --device
levelrail-cli up --with postgres
levelrail-cli open shop
```

`up` reads the directory, works out how to build it, creates the app, creates and connects the database, builds from your git remote and prints the app's URL. Push your code to the remote first: the server builds from git, not from your working copy.

Before it creates anything, `up` asks the remote whether your branch is there at your latest commit. If not, it stops, names the branch and prints the fix, `git push -u origin <branch>`. Pass `--allow-unpushed` to build what the remote has anyway. When the check cannot run (no git, a detached HEAD, an unreachable remote), `up` warns and continues.

`up` then follows the first deploy and exits when the app is healthy: 0 healthy, 7 not healthy (the failure reason is printed), 6 still running when `--timeout` ran out (default 15m, the deploy keeps going). A new domain's certificate may still be pending; `up` says so. Pass `--no-wait` to return as soon as the build is started and follow it yourself with `levelrail-cli apps deploys wait`.

## What `up` infers

Each value comes from the first of these that has one: a flag, your app spec (`app.yaml`), detection, a default. `up` prints every value with where it came from, so you know what to change.

| Value | Flag | App spec | Detection | Default |
| --- | --- | --- | --- | --- |
| Name | `--name` | the service key | the directory name | none |
| Port | `--port` | `port` | Dockerfile `EXPOSE`, or the framework (Node 3000, Python 8000, Go and Java 8080) | none |
| Build | none | `build.type` | Dockerfile, otherwise Railpack, or a static site for a bare `index.html` | none |
| Repository | `--repo` | none | the `origin` remote | none |
| Branch | `--ref` | none | the current branch | `main` |
| Image repo | none | none | none | the app name |
| Replicas, strategy | `--replicas`, `--strategy` | `replicas`, `strategy` | none | 1, blue-green |
| Memory, CPU | `--memory`, `--cpu` | `resources` | none | unlimited |
| Domain | `--domain` | `domains` | none | the automatic URL |

A Node project without a Dockerfile works: it builds with Railpack. A project in a subdirectory of a monorepo (`levelrail-cli up apps/web`) builds from that subdirectory.

## See the plan first

`--dry-run` prints what was inferred and the exact commands `up` runs, and changes nothing:

```bash
levelrail-cli up --dry-run --with postgres --domain shop.example.com --memory 512Mi
```

This output comes from a sample Node.js project with a `main` branch pushed to GitHub:

```text
up: .
  note: detected Node.js (Express)
  name            shop                                     (detected)
  port            3000                                     (detected)
  build           railpack                                 (detected)
  repo            https://github.com/acme/shop.git         (git remote)
  ref             main                                     (git remote)
  image repo      shop                                     (default)
  replicas        1                                        (default)
  strategy        blue-green                               (default)
  domain          shop.example.com                         (flag)
  resources       512Mi                                    (flag)
  database        shop-postgres (postgres)                 (default)
plan (new app):
  levelrail-cli apps create --name shop --port 3000 --repo https://github.com/acme/shop.git --ref main --image-repo shop --build-type railpack
  levelrail-cli apps scale shop --memory 512Mi
  levelrail-cli databases create --name shop-postgres --engine postgres --version <engine default>
  levelrail-cli apps connect shop shop-postgres --env-var DATABASE_URL
  levelrail-cli apps domains add shop shop.example.com
  (up connects databases before the first build starts, so the first container already has its connection env)
change any value with its flag, or commit an app.yaml; see "levelrail-cli up -h"
```

On a terminal, `up` asks for confirmation before it changes anything. Pass `--yes` in scripts.

## Run it again

`up` is safe to repeat. A second run finds the app, database, connection and domain it already made, changes only settings that differ from what you asked for, and rebuilds from your latest push. Nothing is created twice.

If a step fails partway, `up` stops and says what to run next. For example, if the database exists but connecting it fails, no build starts, and the message names `levelrail-cli apps connect` and says re-running `up` is safe.

## If this fails

| Message | What to do |
| --- | --- |
| `no git remote "origin" found in ., so the server has nothing to build from; add one (git remote add origin URL) and push, or pass --repo URL` | Add the remote and push your branch, or pass `--repo`. |
| `app "shop" is set up but the build did not start, re-run "levelrail-cli up" to retry` | The app exists and nothing is built yet. Run `levelrail-cli up` again. |
| `app "shop" and database "shop-postgres" exist but connecting them failed, no build was started` | Run the `apps connect` command the message prints, or run `levelrail-cli up` again. |
| `app "shop" is deployed but adding domain "shop.example.com" failed` | Run the `apps domains add` command the message prints. The app is already running. |

If the build itself fails, the app still exists. Run `levelrail-cli apps diagnose shop` for the likely cause, and see [Deploy failures](/deploy-failures) for each failure code.

## Advanced flags

| Flag | What it does |
| --- | --- |
| `--name`, `--port` | Override the inferred name and port. |
| `--domain` | Add a domain after the deploy, with automatic DNS when a provider is connected. |
| `--with postgres,redis` | Create and connect databases. Use `postgres:16` to pin a version. The first database of a kind is injected as `DATABASE_URL`, `REDIS_URL` or `MONGODB_URL`. |
| `--replicas`, `--strategy` | Replica count and `rolling`, `recreate` or `blue-green`. |
| `--memory`, `--cpu` | Resource limits, for example `512Mi` or `2Gi` and `0.5` cores. |
| `--repo`, `--ref` | Build a different repository or branch. |
| `--secret KEY=VALUE` | Give a secret that your `app.yaml` declares as required. |
| `--dry-run`, `--yes` | Print the plan only, or skip the confirmation prompt. |

For anything `up` does not cover, use the commands it expands to: [`apps create`](/cli-reference), `apps scale`, `apps connect`, `apps domains add`. An `app.yaml` in the project replaces detection, and flags still override it.

## Day to day

```bash
levelrail-cli logs shop
levelrail-cli env set shop LOG_LEVEL=debug --apply
levelrail-cli apps scale shop --memory 1Gi
levelrail-cli rollback shop
levelrail-cli backup shop-postgres
```

`rollback` without `--image` goes back to the previous successful deploy. `backup` uses your only backup target when there is exactly one. Run `levelrail-cli help` for the everyday verbs and `levelrail-cli help advanced` for the rest.

See the [CLI reference](/cli-reference) for every command and the [getting started guide](/getting-started) for the server install.
