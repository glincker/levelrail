---
title: "Developers: API, CLI and MCP server for your Levelrail instance"
description: "Build on Levelrail: the REST API at /api/v1 on your own instance, the levelrail-cli command line, the self-hosted MCP server, and scoped API tokens."
"og:url": "https://docs.levelrail.com/developers"
---

**Build on the API every Levelrail surface already uses**

Levelrail is self-hosted, so there is no hosted public API. Your own instance serves a versioned REST API at /api/v1, and the dashboard, CLI and MCP server all run on it.

[API reference](/api-reference) | [CLI reference](/cli-reference)

## Three ways in, one API

<CardGroup cols={3}>
  <Card title="REST API">
    Versioned routes under /api/v1 on your own control plane, authenticated with an API token. Every route lists the ability it needs.
  </Card>
  <Card title="levelrail-cli">
    A pure HTTP client for the same API: deploy, roll back, read logs and manage nodes from a terminal or a CI job.
  </Card>
  <Card title="MCP server">
    levelrail-mcp exposes the API as MCP tools over stdio or a network listener you run yourself, with read-only and tool profile modes.
  </Card>
</CardGroup>

## Scoped API tokens

Tokens carry abilities such as read, write and deploy, so an agent or CI job gets only what it needs. IAM policies can further allow or deny access to one resource, such as a single app, and an explicit deny always wins.

Pick the narrowest ability set a client actually needs. Details are in Identity and access.

## Route table and spec

An OpenAPI 3.1 document listing every route, its method, path parameters and required token ability is published at https://levelrail.com/openapi.json and regenerated with the API reference. Request and response bodies are not described yet, so they are left generic. Each instance also serves a route table at GET /api/v1/openapi.json.

## Retries and paging

Send an `Idempotency-Key` header (up to 255 characters) on `POST /api/v1/apps`, `POST /api/v1/apps/{name}/deploys` and `POST /api/v1/scheduled-maintenance` to make a retry safe. The first response is stored for 24 hours per caller and key. A repeat with the same body gets that response back with `Idempotent-Replayed: true` and does not run again. A repeat with a different body gets 422. Server errors and rate limit responses are not stored, so those can be retried. Without the header nothing changes.

List routes for apps, nodes, a tag's apps, previews and passkeys take optional `limit` (capped at 500) and `offset`. `X-Total-Count` carries the match count before paging. Without `limit` the whole list returns.

Public status page and deploy badge routes answer errors as `text/plain` on purpose, so they render in an `<img>` or a plain fetch. Every other route answers errors as JSON `{"error": "...", "code": "...", "request_id": "..."}`. `error` is the human message. `code` is a stable machine string: from the HTTP status (`validation_failed`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `rate_limited`, `internal_error`) or a more specific one such as `step_up_required`. `request_id` equals the `X-Request-Id` response header, so quote it when reporting a problem. Some errors add `fix`, a suggested remedy. The CLI prints `fix:` and `request id:` lines on stderr, and `--json` errors carry `api_code`, `request_id` and `fix`.

## Related

- [Identity and access](/identity-and-access)
- [App spec reference](/app-spec-reference)
- [AI assistant integration](/ai-assistant)
- [Working with AI agents](/agents)
- [Getting started](/getting-started)

## Point a tool at your instance

Install Levelrail, create an API token, and call /api/v1 from the CLI, MCP server or your own code.

[API reference](/api-reference) | [CLI reference](/cli-reference)
