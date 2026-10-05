# Levelrail: agent guide

Levelrail is a self-hosted deployment platform: one Go control plane, one Go node agent, Docker's Engine API, embedded Caddy and BuildKit, SQLite state, and a React dashboard embedded in the control plane binary. This file tells a coding agent how to work in this repository. Human contributors should read [CONTRIBUTING.md](CONTRIBUTING.md); the two agree.

## Layout

| Path | What lives there |
| --- | --- |
| `cmd/levelrail` | Control plane binary |
| `cmd/levelrail-agent` | Node agent binary |
| `cmd/levelrail-cli` | CLI, a pure HTTP API client |
| `cmd/levelrail-mcp` | MCP server over the same HTTP API |
| `internal/` | Everything else: `api`, `reconcile`, `store`, `docker`, `build`, `ingress`, `agent`, `catalog`, `mcptools`, `brand` |
| `web/` | Vite, React, TypeScript, Tailwind dashboard |
| `docs/` | VitePress site, served at levelrail.com |
| `adr/` | Architecture decision records. Read the relevant one before changing a locked decision |
| `scripts/` | Hooks, test runners, generators, `screenshots/capture.sh` |

## Commands

```
go build ./cmd/levelrail ./cmd/levelrail-agent ./cmd/levelrail-cli ./cmd/levelrail-mcp
scripts/install-hooks.sh                 # once per clone or worktree
scripts/affected-tests.sh                # tests for what you changed, plus dependents
go test -short ./internal/foo            # one package
scripts/smoke.sh -- nodes list -- attention   # boot a real dev server, drive the real CLI
golangci-lint run
cd web && npm ci && npx tsc -b && npx vitest run --changed
cd web && npx vite build                 # also regenerates the route tree
cd docs && npm run check:descriptions
```

Use `npx tsc -b`, not `--noEmit`: the root web tsconfig lists no files, so `--noEmit` checks nothing. Do not run the full `go test ./... -race` while iterating; CI and the nightly run do that.

## Rules the hooks and CI enforce

- Conventional commits: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`, `ci`, `security`. Subject plus at most three short body lines.
- No em dashes or en dashes anywhere: code, comments, docs, commit messages, PR text. Use commas, periods, colons, or parentheses.
- Comments default to none. Add one only for a non-obvious why, in one to three lines. Never narrate history or restate the code.
- Never commit to `main`. Branch, open one PR per coherent change, and say what the PR does not do.
- No `any` or `interface{}` in exported Go signatures, and no `any` in TypeScript. Wrap errors with context at package boundaries, and pass a context to every blocking call.
- User-visible product and binary names come from `internal/brand`, never from string literals.
- Dashboard strings go through `useTranslation()` and `web/src/locales/en/*.json` (see `docs/i18n.md`). Icons are Phosphor only (ADR 014). Styling is Tailwind tokens, not inline styles.
- SQL migrations need unique, increasing version numbers. Check the latest on a fresh `origin/main` before you push (`scripts/check-migration-versions.sh`).
- Code under `internal/reconcile` is level-triggered and idempotent. Every reconciler needs a test for the half-succeeded case.
- A feature is not done until it is reachable from the dashboard and the CLI as well as the API.

## Docs, generated files, and screenshots

- Every page under `docs/` needs a `description` in its frontmatter. Do not use em dashes, and keep counts and tool numbers generated, not typed.
- `docs/template-catalog.md`, `docs/mcp-tool-surface.md`, the API reference and `docs/public/openapi.json` are generated. Run the generator (`go run ./scripts/gen-template-catalog-docs`, `go run ./scripts/gen-api-reference`, `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`) instead of editing them.
- `docs/public/llms.txt` is the curated index of pages for language models, and `llms-full.txt` is built from it. Add a new page there.
- Dashboard screenshots in `docs/assets/screenshots/` come from `scripts/screenshots/capture.sh` against a real instance. Check each image before embedding it.

## Using Levelrail from an agent

This section is for an agent that deploys or operates an app on a Levelrail instance, not for one changing this repository.

- Run `levelrail-cli init` in a project to write `app.yaml`, an `AGENTS.md` with deploy instructions, and an `.mcp.json` for `levelrail-mcp`.
- The skill in [skills/levelrail](skills/levelrail/SKILL.md) covers the deploy, wait, diagnose and roll back loop. Install it with `npx skills add glincker/levelrail`.
- `levelrail-mcp` exposes tools by mode (`agent-core`, `read-only`, `standard`, `full`). Start with the smallest mode that does the job. See [docs/agents.md](docs/agents.md) and [docs/mcp-tool-surface.md](docs/mcp-tool-surface.md).
- Documentation for models: `https://levelrail.com/llms.txt` (index) and `https://levelrail.com/llms-full.txt` (full text).
- Never write an API token into a file, a commit, or a chat message. Tokens come from `APP_API_TOKEN`, and the instance URL from `APP_API_URL`.
