---
title: "Monorepos"
description: Detect workspaces, run one service per package from a single repository, and redeploy only the services a push actually changed.
"og:url": "https://docs.levelrail.com/monorepos"
---

A repository with several services should not rebuild all of them on every push. Detection lists the workspace packages in a connected repository, and each package carries a ready-made set of watch paths so a push only redeploys the services it touches.

## What detection finds

Detection reads the provider's file tree (no clone on GitHub, GitLab and Gitea) and recognises:

| Tooling | Where members come from |
| --- | --- |
| pnpm (and turbo on top of it) | `packages:` globs in `pnpm-workspace.yaml`, with `!` exclusions |
| npm, yarn, bun | `workspaces` array or `{ "packages": [] }` in the root `package.json` |
| Go | `use` directories in `go.work` |
| Nx, Lerna, Cargo | reported as tooling; members are found through the globs above |

A directory counts as a package when it matches a workspace glob and holds a `package.json`, `go.mod` or `project.json`. Up to 60 packages are listed.

## Watch paths per package

Each package gets a deploy filter made of:

- its own directory (`services/web/**`),
- the directory of every workspace package it depends on (read from `dependencies` and `devDependencies` in its `package.json`),
- the root files that change every install or build: lockfiles, `package.json`, `pnpm-workspace.yaml`, `turbo.json`, `nx.json`, `tsconfig.base.json`, `go.work`, `go.mod` and `go.sum`.

So a change under `services/api` skips the `web` service, a change under a shared package redeploys only its dependents, and a root lockfile change redeploys everything. The filter is the same doublestar `deploy_paths` list described in [Git integrations](/git-integrations); an empty diff never matches.

## From the dashboard

Open the app's **Source** tab, edit the build settings, and look under **Workspace packages**. **Use this package** sets the package directory as the build root (with its Dockerfile when it has one) and saves its watch paths. To run several services from one repository, connect the same repository to one app per package and apply a different package to each.

The same panel can dry-run a change: paste changed file paths, one per line (list both names of a renamed file), and each package shows **Redeploys** or **Skipped**, together with the verdict for this app.

In the new app wizard, the **Advanced** section has a **Validate directory** button next to the build context directory. It re-runs detection for the typed directory and warns when that directory does not exist on the branch, before anything is created.

## From the CLI

```bash
levelrail-cli apps git-source detect web
levelrail-cli apps git-source detect web --package services/web
levelrail-cli apps git-source affected web --files services/api/index.js,pnpm-lock.yaml
```

`--package` applies the build root and watch paths in one step. `affected` is read only.

## API

`POST /api/v1/apps/{name}/git-source/detect` returns a `packages` array (`name`, `dir`, `tool`, `dockerfile`, `depends_on`, `watch_paths`). `POST /api/v1/apps/{name}/git-source/affected` takes `{ "files": [...] }` and returns whether this app deploys and which packages are affected.

## Import plan

Before creating anything, ask for a plan: one proposed app per workspace package, with a name checked against your existing apps (a taken or duplicate name gets a `-2` style suffix and a warning), the build context directory, the detected port or none, the build kind (`dockerfile`, `buildpack` or `static`), watch paths, `depends_on` and warnings (`no_dockerfile`, `no_port`, `duplicate_port`, `duplicate_name`, `name_taken`, `package_cap`). It is read only and creates nothing.

```bash
levelrail-cli import repo plan https://github.com/acme/mono --branch main --dir apps
levelrail-cli import repo plan https://github.com/acme/mono --json
```

`POST /api/v1/imports/repo/plan` takes `repo_url`, optional `branch` and `base_directory` (only packages under it are planned) and an optional one-shot `token` for a private repository that is never stored or returned. It needs the write ability. In the New app wizard the same plan appears as a **Preview the plan** panel above the package list.

The number of packages listed is capped by `APP_WIZARD_MAX_PACKAGES` (default 60); the plan reports how many were found. `APP_WIZARD_PLAN_CONCURRENCY` (default 4) bounds how many packages are analyzed at once against the git host.

## Create the apps from the plan

The wizard's plan panel (**Monorepo**) is editable: tick the packages to include, change an app name or port, add environment variables per app (mark a value **Secret** to store it encrypted), and pick a project and environment. **Check N apps** runs a dry run that lists every problem per app and creates nothing. **Create N apps** then makes the apps on the server, so closing the tab does not stop it, and a results screen shows each app's status with a link and a **Retry** for the ones that failed.

```bash
levelrail-cli import repo apply https://github.com/acme/mono --dry-run
levelrail-cli import repo apply https://github.com/acme/mono --packages api,web \
  --set api.LOG_LEVEL=debug --secret api.DB_PASSWORD=... --project proj_1 --environment env_1 --yes
```

`POST /api/v1/imports/repo/apply` takes `repo_url`, `branch`, `base_directory`, an optional one-shot `token`, `packages` (`package`, `name`, `directory`, `port`, `env`, `secret_env_keys`, `project_id`, `environment_id`, `deploy`), `dry_run` and `confirm`. The server rebuilds the plan itself, so directories and build kinds are never taken from the request. Each app is created through the same code as a single app, so name, port, project and quota checks, secret storage and permissions are identical: the caller needs the write ability, plus the sensitive-write ability to connect the repository and the deploy ability when `deploy` is set.

- Apps are created in dependency order. If one fails, apps that depend on it are reported as `skipped_dependency` and the independent ones carry on.
- Re-running is safe: an app whose name already tracks the same repository and directory is reported as `already_exists` and left alone. Any other app using the name is `failed` with `name_taken`.
- Per-app status is `created`, `already_exists`, `skipped_dependency`, `failed` or `would_create` (dry run), with an error `code` and `message`. Secret values are never returned.
- Importing more than `APP_WIZARD_IMPORT_CONFIRM_ABOVE` apps at once (default 10) needs `confirm: true` (`--yes` in the CLI, a confirmation dialog in the dashboard); without it the request is refused with `confirmation_required` and nothing is created.
- Protected environments and static sites are refused per app with `protected_environment` and `unsupported_build`; create those from the single app flow.
- A private repository's token is passed to each created app's repository link the same way a single app stores it, and is not echoed or logged.

## Try it

`test/fixtures/monorepo` is a pnpm workspace with two tiny services and a Dockerfile each.

## One app per package

In the New app wizard, a monorepo lists its packages. Select several and press **Create an app for each selected package**: each app gets the package directory as its build root and its watch paths. `levelrail-cli apps analyze REPO_URL` lists the packages too. See [Import a repository](/import-a-repo).

## Limits

- Dependencies are followed for Node packages only; Go modules are matched by directory and root files.
- Watch paths are saved on the app when you apply a package, so editing the workspace later means applying the package again.
- Push events whose changed-file list the provider does not return deploy unfiltered.
