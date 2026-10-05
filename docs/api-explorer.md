---
description: The in-dashboard API explorer under Settings, where signed-in operators browse every control plane route and try requests against their own instance.
---

# API explorer

The API explorer lists every HTTP route the control plane registers and lets you fire a real request at it from the browser. Use it to find out what the API offers, or to prototype a script or integration before writing any code.

Open **Settings > API explorer** (`/settings/api-explorer`). You need to be signed in.

## What it shows

- Every registered route, grouped by resource as in the [API reference](/api-reference).
- Each route's method, path, required ability and, where one exists, a short description.
- A filter box that matches path, method, ability and description.
- A **Try it** form per route. It sends the request to this instance with your current browser session. Path parameters such as `{name}` become text fields, and the raw JSON response is shown exactly as the API returned it. The request is real: a `POST` or `DELETE` changes your instance.
- A worked request and response for a small set of common routes (apps, brand, system status).

Most routes have no worked example and about three quarters have no description. The page header shows the current counts.

## Where the data comes from

The explorer reads `GET /api/v1/openapi.json`, which needs the `read` ability (a session or a token). Despite the name it is not a full OpenAPI document: it is a subset with method, path, ability, group and description for each route, generated from the route registrations at build time. The static [API reference](/api-reference) is generated from the same source, so the two always list the same routes.

For calling the API from outside the dashboard, create a token as described in [Identity and access](/identity-and-access). AI agents reach the same API through the [MCP tool surface](/mcp-tool-surface).
