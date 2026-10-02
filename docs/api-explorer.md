---
description: In-app interactive API explorer for logged-in operators, under Settings.
---

# API explorer

An authenticated, in-dashboard way to browse and try the control plane's real HTTP API, for an operator exploring what's available or someone prototyping an MCP tool or other integration against this platform, without leaving the browser or reading `docs/api-reference.md` side by side with `curl`.

Reach it at **Settings > API explorer** (`/settings/api-explorer`).

## What it shows

- Every registered route, grouped by resource exactly like `docs/api-reference.md`'s own groups.
- Each route's method, path, required ability, and (where a source comment documents it) a short description.
- A "Try it" form that fires the real request against this instance using your current browser session, the same cookie every other settings page already relies on. Path parameters (`{name}`, `{id}`, ...) become text fields; the raw JSON response is shown as returned, not reformatted into something prettier than the API actually sends.
- A worked request/response example for a small set of commonly used routes (apps, brand, system status). Most routes have no example, only method/path/ability/description; that gap is deliberate, not a bug, see below.

## Data source

The explorer is served from `GET /api/v1/openapi.json`, gated the same way `GET /api/v1/system/status` is (session or token with `read`). The response is **not** a full OpenAPI 3.1 document: it's a pragmatic subset (method, path, ability, group, and description) generated at build time by `scripts/gen-api-reference` straight from the `mux.HandleFunc` registrations in `internal/api/routes*.go`, the same source `docs/api-reference.md` is generated from. Request/response examples for a handful of routes are hand-written in `internal/api/openapi.go`, not generated.

Coverage is honest, not exhaustive:

- Every route gets method, path, ability, and group, always.
- A route only gets a description when its registration in `routes*.go` has a doc comment directly above it. Roughly a quarter of routes do.
- A route only gets a worked example when it's one of the handful hand-written into `openAPIExamples`.

If you change a route's registration, run `go run ./scripts/gen-api-reference` and commit the result; it updates both `docs/api-reference.md` and `internal/api/openapi_gen.go` together, and `TestAPIReferenceCoversEveryRoute` fails CI if you forget.

See also: [API reference](/api-reference) for the same route table as static docs, and [MCP tool surface](/mcp-tool-surface) for how an AI agent reaches this API through a higher-level tool layer instead.
