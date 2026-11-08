---
title: "Doctor and import from the command line"
sidebarTitle: "Doctor and import CLI"
description: Run the server checks, apply safe fixes, explain a failing check, and plan or create apps from a repository, an image, a config folder or a monorepo with levelrail-cli.
"og:url": "https://docs.levelrail.com/doctor-and-import"
---

Two command groups do most of the work from a terminal. `doctor` checks the server and explains what is wrong. `import` turns a repository, an image or a config folder into an app. The dashboard versions are on [Doctor](/doctor) and [Import a repository](/import-a-repo). This page covers the CLI flags, output and exit codes.

## Doctor

### Run the checks

```bash
levelrail-cli doctor
levelrail-cli doctor --output json
```

This runs the same checks as the Doctor page in the dashboard. The checks cover Docker, disk and inodes, ports, firewall, clock, DNS, certificates, backups and restore drills, node agents, the WireGuard mesh, orphaned Docker data and dashboard HTTPS.

Each check has a status, a message and, when there is a problem, a fix ID and fix kind. `--output json` returns `ok` (true when no check fails) and the `checks` list.

Exit codes depend on the output format:

- With the default table output, the command exits `1` when any check fails and `0` otherwise.
- With `--json`, `--output json` or `--output text`, the command exits `0` even when checks fail. Read the `ok` field instead.

### Apply safe fixes

```bash
levelrail-cli doctor --fix
levelrail-cli doctor --fix --yes
```

`--fix` builds a plan from the failing and warning checks that offer an automatic fix. It prints the plan and asks `Apply N fix(es)? [y/N]`. `--yes` applies without asking.

- Only fixes that are automatic and marked `safe` are applied.
- Any other fix is listed as skipped with the reason. A fix with a higher risk is applied on purpose with `levelrail-cli fixes apply <fix-id>`, which can also take `--param k=v`.
- A fix the server reports as rate limited or demoted is skipped with the server's reason.
- Without a terminal, `--fix` refuses to run unless you pass `--yes`.
- After applying, the report runs again and the result is printed.

`--fix` needs a control plane. With none configured, it stops with an error.

### Explain one check

```bash
levelrail-cli doctor explain <check-id>
levelrail-cli doctor --output json --query "checks[?status=='fail'].check_id"
```

The second command lists the IDs of failing checks, so you can pass one to `explain`.

`doctor explain <check-id>` prints what is wrong, why it matters and the next step, plus the fix ID when one exists. The check ID is the `check_id` field in the report. The explanation is a fixed table, so the command does not contact the network.

### Local checks without a control plane

```bash
levelrail-cli doctor --local
```

`--local` runs only the Docker, disk and port checks on this host and never contacts a control plane. The same happens automatically when no server is configured (no `--api-url`, no token, no profile). In table mode, the CLI says on stderr that it is running local checks only.

The local disk warning threshold is 1 GiB free. Set `APP_DOCTOR_DISK_WARNING_BYTES` to change it, in bytes.

### Disk only

```bash
levelrail-cli doctor disk
```

This is read-only. It shows the disk checks only: `disk_space`, `docker_root_disk`, `disk_hygiene`, `inodes` and `docker_orphans`. It lists the largest consumers and a cleanup command it suggests but never runs. The control plane warning level for free space is set by `APP_DOCTOR_DISK_WARN_FREE_PCT` (percent free).

### Network, firewall and paths

```bash
levelrail-cli doctor reachability
levelrail-cli doctor cloud
levelrail-cli doctor paths
```

- `reachability` lists the listening ports. When the dial-back setting is on, it also shows which ports the internet cannot reach.
- `cloud` (also `doctor --cloud`) prints the provider firewall rules this host needs, with the console link for each provider. It runs offline.
- `paths` runs offline. It prints the binary, releases directory, data directory and systemd unit locations, and the address the unit binds (`APP_HTTP_ADDR`).

### Domain checks

```bash
levelrail-cli domains doctor <app> <domain>
levelrail-cli domains summary
```

`domains doctor` runs every check for one domain: DNS per resolver, CAA, ports 80 and 443, the TLS certificate, the HTTP status, redirects, HSTS, proxy ownership, app state and certificate renewal. It prints a fix list in priority order. It only connects to the domain when the domain resolves to this server. Otherwise it runs the DNS checks and says why it stopped there.

`domains summary` counts domains by status and certificates that are expiring or failing to renew.

### App failures

```bash
levelrail-cli apps diagnose <name>
```

This explains why the latest deploy failed or why the app is crash looping. It applies fixed rules to signals the platform already collected, such as the deploy error, reconcile reasons and recent log lines. When no rule matches, it says so. Nothing changes unless you pass `--apply-fix N`.

## Import

The plan, analyze and apply commands need a control plane. `import config` is offline.

### Plan a source

```bash
levelrail-cli import https://github.com/you/app
levelrail-cli import nginx:1.27
levelrail-cli import -f docker-compose.yml
levelrail-cli import -f Dockerfile
cat docker-compose.yml | levelrail-cli import -f -
levelrail-cli import --docker-run "docker run -p 8080:80 nginx:1.27"
```

Give exactly one source: a repository URL or image argument, `-f` with a file (or `-` for stdin), or `--docker-run` with a command. The input is classified as a GitHub, GitLab, Gitea or Bitbucket repository URL, a `docker run` command, an image reference, a `docker-compose.yml` or a Dockerfile. The command prints the plan. Nothing is created.

Useful flags: `--name` (default derived from the source), `--ref` (branch for a repository), `--port` (container port override) and `--json`.

### Create the app

```bash
levelrail-cli import https://github.com/you/app --name web --ref main --port 3000 --deploy
levelrail-cli import -f compose.yaml --env API_KEY=replace-me --deploy
```

`--deploy` creates the app and deploys it after the plan prints.

If the plan lists required variables with no value:

- In a terminal, the CLI prompts for each one. Secret values are read without echo.
- Without a terminal, the command stops and lists the keys. Pass each one with `--env KEY=VALUE`.

### Analyze or apply a repository

```bash
levelrail-cli import --analyze https://github.com/you/app --ref main
levelrail-cli import --apply https://github.com/you/app --ref main --name web --no-database
```

- `--analyze` shows the detected stack, build, port, health check, environment keys and databases, each with its evidence. It creates nothing.
- `--apply` creates the suggested databases, the app, the repository link and the first build. `--no-database` skips the databases.
- Overrides for both: `--base-directory DIR` for a monorepo app, `--dockerfile PATH`, and `--build-type dockerfile|railpack|static`.

### Preview a config folder

```bash
levelrail-cli import config ./my-app
levelrail-cli import config ./my-app --out app.yaml --strict
levelrail-cli import config ./my-app --emit stack --out ./review --db-version postgres=16
```

`import config` reads `Procfile`, `app.json`, `vercel.json` and `CHECKS` from a folder and prints a proposed `app.yaml`. Findings go to stderr, blockers first, each with a fix. It needs no token and creates nothing.

- `--out FILE` writes the proposed file instead of printing it.
- `--strict` exits `2` when any finding is a blocker.
- `--json` prints `detected`, `spec` and `findings` as JSON.
- `--emit stack --out DIR` writes Stack, Project, Database, App and Domain documents, a `SECRETS.md` listing secret names only, and a `MIGRATION.md`. Review them in a pull request. `--force` overwrites existing files.

The same translation runs at deploy time for a repo that has a `Procfile` and no app spec: one service per process type, `release:` as a pre-deploy hook, a `Dockerfile` build when one exists, and `app.json` healthchecks, cron, scripts and formation mapped to health, jobs, hooks and replicas. Unsupported keys come back as findings in the deploy log. A `CHECKS` file in the repo root becomes a readiness probe when `app.json` sets none; lines it cannot express come back as findings. See the Procfile and app.json section of the app spec reference.

### Monorepo packages

```bash
levelrail-cli import repo plan https://github.com/you/monorepo --branch main
levelrail-cli import repo plan https://github.com/you/monorepo --dir apps
levelrail-cli import repo apply https://github.com/you/monorepo --dry-run
levelrail-cli import repo apply https://github.com/you/monorepo --packages web,api --deploy
levelrail-cli import repo apply https://github.com/you/monorepo --set web.PUBLIC_URL=https://example.com --secret api.DATABASE_URL=replace-me --yes
```

`import repo plan` lists one proposed app per workspace package: name, directory, port, build kind, dependencies and warnings. It creates nothing. `--branch` picks the branch to inspect, and `--dir` plans only the packages under one directory.

`import repo apply` plans the repository, then creates one app per package:

- `--packages a,b` limits the packages. `--project` and `--environment` place the apps.
- `--set PACKAGE.KEY=VALUE` sets an environment value. `--secret PACKAGE.KEY=VALUE` sets a secret one.
- `--dry-run` checks everything and creates nothing.
- When the import creates more apps than the server's confirmation limit, the command stops and asks for `--yes`.
- A failed app does not stop independent ones. Apps that depend on a failed app are skipped. Running the command again skips apps that already track the same package.
- `--deploy` starts the first build of each app.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The command succeeded. |
| `2` | `import config --strict` found a blocker, or `import repo apply` failed or skipped an app. |
| `3` | The API could not be reached. |
| `4` | The API answered with an error. |

## Not yet

- A `doctor` exit code of `1` for failed checks in JSON and text output. Today those formats exit `0`, so scripts must read the `ok` field.
- A way to run `doctor --fix` for risky fixes without `fixes apply`.
