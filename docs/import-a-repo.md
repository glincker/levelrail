---
title: "Import a repository"
sidebarTitle: "Import a repo"
description: Go from a git repository to a running app in a few steps, with detection that tells you what it found and why.
"og:url": "https://docs.levelrail.com/import-a-repo"
---

Pick a repository and Levelrail reads its file tree through the provider API (it never clones or runs your code), explains what it found, asks only for what it cannot guess, and then creates the app and starts the first build. Nothing exists until you press Deploy.

## The flow

In the dashboard, **New app** is one page with three calm steps:

1. **Where is your code.** Paste a repository URL or pick one from a connected account. There is no Analyze button: the preview starts a moment after you stop typing.
2. **Preview.** A repository card appears in place with the owner and name, visibility, description, a languages bar, the latest commit, stars, forks and license, a README excerpt (plain text, never rendered as HTML) and a branch select that defaults to the default branch. Below it, one line says what was detected (for example "Next.js, builds with Railpack, listens on 3000") with an **Edit** shortcut, and a monorepo gets an app picker. A private repository without a connected account explains that in one line and offers **Connect an account** or **Use a deploy token**. If the host is rate limited or has no preview, you can still continue.
3. **Short form.** App name (suggested from the repository, checked against your existing apps), only the environment variables the repository needs (secret-looking names are masked, **Add variable** for more), and the address (a free address by default, or a custom domain). A summary line such as "Builds with Railpack from main and deploys to a free address" sits above the single **Deploy** button. Port, health check path, memory, directory and build type live under **Advanced**, with the detected values shown as placeholders.

Deploy creates the databases, the app and its connections, links the repository and starts the first build. You land on the live deploy view with the step trace and logs. Environment variable values are never read from the repository, so detected variables start empty.

### Preview endpoint

`POST /api/v1/git-sources/preview` takes `repo_url`, optional `ref`, `account_id` and `token`, needs the same write ability as analyze, and returns the card data plus the analysis. `kind` is one of `public`, `private_needs_account`, `not_found`, `rate_limited`, `unsupported_host` or `invalid_url`. Only github.com, gitlab.com, bitbucket.org, codeberg.org and the hosts of your connected providers are contacted, through the same SSRF-guarded client as the rest of the git layer, so URLs such as `http://169.254.169.254` or `http://localhost` are answered `unsupported_host` without any request. Tokens are never returned or logged. Tune it with `APP_REPO_PREVIEW_TIMEOUT`, `APP_REPO_PREVIEW_CACHE_TTL`, `APP_REPO_PREVIEW_CACHE_MAX`, `APP_REPO_PREVIEW_MAX_BRANCHES` and `APP_REPO_PREVIEW_README_CHARS`. Bitbucket Cloud has no rich preview yet; it falls back to the classic analysis step.

The flow survives a page reload: the repository and step are kept in `sessionStorage` (never tokens or env values), and the analysis is simply run again.

## Explain it from the CLI

```bash
levelrail-cli apps analyze https://github.com/you/app [--branch main] [--dir apps/web]
```

It prints the detection and every reason behind it, and creates nothing. `--output json` returns the full result, including the `detection` object:

| Field | Meaning |
| --- | --- |
| `framework`, `language` | What the repository is, for example `Next.js` and `node`. |
| `build_type` | `dockerfile`, `railpack`, `static` or `compose`. A Dockerfile always wins over auto-detection. |
| `confidence` | `high`, `medium` or `low`. Low means a generic guess; check it. |
| `reasons` | `{ signal, detail }` pairs such as `package.json has next in dependencies`. |
| `port` | The value and where it came from: a Dockerfile `EXPOSE`, a package script, or the framework default. |
| `start_command`, `build_command`, `output_dir` | Suggested commands, and the folder a static build produces. |
| `health_path` | A Dockerfile `HEALTHCHECK` path, the framework's own (`/up` for Rails, `/actuator/health` with Spring actuator), or `/`. |
| `runtime_version` | Node from `engines.node`, `.nvmrc` or `.node-version`; Python from `.python-version`, `runtime.txt` or `pyproject.toml`; Go from `go.mod`; Ruby, PHP, Rust and Java from their own manifests. |
| `env_vars` | Names only, with where they were found, whether they look required, and a `secret_guess`. |
| `warnings` | Codes such as `no_lockfile`, `port_unknown`, `multiple_dockerfiles`, `dockerfile_no_expose`, `malformed_manifest` and `monorepo_no_root_app`. |

The same result comes from `POST /api/v1/git-sources/analyze` (body: `repo_url`, optional `ref` and `directory`), which reuses your connected provider credentials. It reads only and persists nothing. The older `POST /api/v1/imports/analyze` returns the same payload.

## What is detected

Next.js (App Router or Pages Router, `standalone` output, static `export`), Vite and Create React App single page apps, Astro (static or with the Node adapter), SvelteKit (static adapter or Node), Nuxt, Remix, Express, Fastify, NestJS, Django, FastAPI, Flask, Rails, Laravel, Go `net/http`, Rust axum, Spring Boot, plain static HTML, and any repository with a Dockerfile.

When signals conflict, the most specific framework wins: a Rails app with a `package.json` for its assets is Rails, and an Express server with a Dockerfile is built from the Dockerfile. A repository with several Dockerfiles uses the shallowest one and warns. A manifest that cannot be parsed falls back to matching its raw text, lowers the confidence, and warns.

<Note>
Rust has no automatic build here yet. Detection still names it and warns; add a Dockerfile and it is built from that.
</Note>

## Monorepos

When the repository is a workspace (pnpm, npm, yarn, bun, Go workspaces, Nx or Turborepo), the **What we found** step lists its packages. Select the ones you want and press **Create an app for each selected package**. Each becomes its own app with the package directory as the build root and its watch paths applied, so a push only redeploys what it changed. You can also press **Configure this one** to take a single package through the normal steps. See [Monorepos](/monorepos).

Required environment variables are not asked for in the bulk action; add them on each app and redeploy.

## Worked examples

These are the fixtures under `test/fixtures`. Output is abridged.

### `smoke/nextjs`: a Dockerfile around Next.js

```text
Framework:  Next.js (node)
Build type: dockerfile
Confidence: high
Port:       3000 (EXPOSE in Dockerfile)
Why:
  - [lockfile] package-lock.json is present, so npm installs dependencies
  - [package.json] package.json has next in dependencies
  - [directory] an app/ directory exists, so the App Router is in use
  - [dockerfile] Dockerfile exists, so the Dockerfile is used instead of auto-detection
```

### `dogfood-nextjs`: standalone output

Same as above, plus `next.config.mjs sets output: 'standalone', so the server starts from .next/standalone`. Port 3000, health `/`.

### `smoke/goapi`: Go with a Dockerfile

```text
Framework:  Go (net/http) (go)
Build type: dockerfile
Port:       8080 (EXPOSE in Dockerfile)
Runtime:    1.22 (go.mod go directive)
warning: no lockfile was found, so installs are not reproducible; commit one
```

A Go module with no dependencies has no `go.sum`; the warning only appears when `go.mod` has a `require`.

### `smoke/static`: a Dockerfile serving static files

The Dockerfile decides: build type `dockerfile`, port 8080 from `EXPOSE`, no framework named. Remove the Dockerfile and the same files are detected as `Static site`, build type `static`, served on port 80.

### `monorepo`: a workspace with no app at the root

```text
Build type: dockerfile
Confidence: medium
warning: several Dockerfiles were found; using services/api/Dockerfile, pick another if it is wrong
warning: this is a monorepo with no app at the root; create one app per package
Workspace packages:
  - api (services/api)
  - web (services/web)
```

Use **Create an app for each selected package** to get one app per service.

### `failing-build`: no recognisable framework

```text
Framework:  Node.js (node)
Build type: dockerfile
Confidence: medium
warning: the Dockerfile has no EXPOSE; set the port the app listens on
warning: no listening port could be detected; set the port the app listens on
```

Detection explains the setup; the build still fails at its own `RUN` step, which is what this fixture exists to show. See [When a deploy fails](/deploying-apps#when-a-deploy-fails).

## Limits

- Analysis reads at most a few dozen small files per repository (`APP_WIZARD_MAX_FILES`, `APP_WIZARD_MAX_FILE_BYTES`), so a huge tree can be partly read; a warning says so.
- It works through the GitHub, GitLab and Gitea APIs. A host without a file API can still be connected by URL, but analysis answers that it cannot read it.
- Port and health guesses for Express and Fastify are conventions, not facts. Check them against the server code.

Related: [Deploying and managing apps](/deploying-apps), [Git integrations](/git-integrations), [Monorepos](/monorepos).
