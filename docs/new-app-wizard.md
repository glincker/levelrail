---
title: "New app wizard"
description: How the New app wizard reads a repository, pre-fills build, port, health path, env keys and databases with evidence, and deploys with one click.
"og:url": "https://docs.levelrail.com/new-app-wizard"
---

The New app page (`/apps/new`) turns a repository into a running app without making you fill in blank forms. It reads the repository, suggests every setting along with the evidence behind it, and creates nothing until you press Deploy. The same analysis is available from the CLI and the HTTP API.

## The three steps

1. **Where is it from.** Pick a repository from the connected GitHub, GitLab, Gitea or Bitbucket app (searchable, with language and last push), paste a git URL (optionally with a one-shot access token for a private repository), or switch to the "Image, compose or docker run" tab or the [template catalog](/templates-and-registry).
2. **What we found.** Every item is an editable, pre-filled suggestion with a line of evidence, for example "found Dockerfile at apps/web/Dockerfile" or "Next.js default port".
3. **Review and deploy.** A plain summary and a list of everything that will be created (app, repository link, databases, domain, environment variables), then one Deploy button that opens the live build log.

Advanced options (build type, Dockerfile path, build context directory, memory limit) are collapsed by default.

## What the analysis finds

| Suggestion | Evidence used |
| --- | --- |
| Framework and runtime | `package.json` dependencies, `go.mod`, `requirements.txt`, `pyproject.toml`, `Gemfile`, `composer.json`, `mix.exs`, `pom.xml`, `index.html` |
| Build type, Dockerfile and context | Dockerfiles in the tree and workspace tooling (turbo, pnpm, nx, lerna), the same detection the build settings card uses |
| Listening port | Dockerfile `EXPOSE`, a `--port` flag in `package.json` scripts, then the framework default |
| Health check path | Dockerfile `HEALTHCHECK` URL, then the framework default (`/up` for Rails, `/` otherwise) |
| Environment variable names | `.env.example`, `.env.sample`, `.env.template`, `.env.dist`, compose `environment` blocks, README code blocks and tables |
| Memory limit | A per runtime default, 768 MiB for Next.js |
| Databases | `DATABASE_URL`, `REDIS_URL`, `POSTGRES_*`, `MYSQL_*`, `MONGODB_*` names, driver dependencies, and database images in a compose file |
| App name and domain | The repository name (plus the app directory in a monorepo), and `<name>.<apps base domain>` when one is configured |

A suggested database is shown with a "Create and attach" switch. On deploy it is created (or reused when a database with that name exists) and attached through the same connection mechanism as Apps > Databases, so its connection URL lands in the variable the repository expects.

In a monorepo with several buildable apps, an app picker appears. Choosing another entry re-runs the analysis for that app, so the framework, port and variables follow the choice.

## Safety

- Files are read through the provider API (GitHub, GitLab, Gitea) with the SSRF-guarded client. Nothing is cloned for the analysis, and a host without a supported file API is refused rather than cloned.
- Reads are bounded: `APP_WIZARD_MAX_FILES` (default 24 files), `APP_WIZARD_MAX_FILE_BYTES` (default 262144 per file) and `APP_WIZARD_MAX_ENV_KEYS` (default 60 names). When the budget is reached the analysis says so.
- Only variable names are returned. Values from example env files, compose files and README text are never imported, and a name that looks like a secret is marked secret. A required variable stays blocking until you type a value.
- The one-shot access token in the URL tab is used for this request only and is not stored by the analysis. Connect the repository through a provider app to keep deploys working after the first build.
- Memory defaults can be tuned per runtime with `APP_WIZARD_MEMORY_MB_<RUNTIME>` (`NODE`, `PYTHON`, `GO`, `RUBY`, `PHP`, `ELIXIR`, `JAVA`, `STATIC`).

## CLI

```
levelrail-cli import https://github.com/acme/shop --analyze
levelrail-cli import https://github.com/acme/shop --apply --env NEXTAUTH_SECRET=...
```

`--analyze` prints each finding with its evidence (or JSON with `--json`) and creates nothing. `--apply` runs the analysis and then creates the databases, the app, the database connections, the repository link and the first build, in that order. Overrides: `--name`, `--port`, `--ref`, `--env KEY=VALUE`, `--base-directory`, `--dockerfile`, `--build-type`, and `--no-database` to skip the suggested databases. A required variable with no value blocks `--apply` before anything is created.

## API

`POST /api/v1/imports/analyze` needs the write ability and creates nothing.

```json
{"repo_url": "https://github.com/acme/shop", "ref": "main", "base_directory": "", "dockerfile_path": "", "build_type": "", "name": "", "token": ""}
```

Only `repo_url` is required. Without `ref` the endpoint tries `main`, then `master`. The response carries `name`, `stack`, `build` (with `alternatives`), `port`, `health`, `memory`, `domain`, `env`, `databases` and `warnings`, each with an `evidence` string.

The dashboard and CLI deploy by calling the existing endpoints in order: `POST /api/v1/databases`, `POST /api/v1/apps`, `POST /api/v1/apps/{name}/connections`, the provider's use-as-source call (or `PUT /api/v1/apps/{name}/git-source`), then `POST /api/v1/apps/{name}/builds`. A failure names its step. Nothing is rolled back; pressing Deploy again resumes after the app record exists, and an existing database with the same name is reused.

## Not covered

- Compose files with several services are imported through the existing import plan, not analysed here.
- Env values are never discovered. Secrets still have to be typed in.
- Health path and port are guesses from files, not from running the app. The first deploy's readiness probe is the check.

## Deploy from a URL

`levelrail-cli deploy <git-url>` runs the same analysis and `--apply` in one step. A positional argument that is a git URL (https, ssh, or `git@host:owner/repo`) selects it; a directory path still uploads that directory. Pasted `.git` suffixes, `git@` forms and `/tree/<branch>/<dir>` links are normalized to the repository, branch and app directory. Use `--dry-run` to print the plan only, and `--branch`, `--name`, `--port`, `--env`, `--base-directory` to override.
