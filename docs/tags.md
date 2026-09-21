---
description: Label and organize apps with arbitrary tags for filtering and grouping.
---

# Tags: organizing apps

Tags are arbitrary labels you attach to apps to organize them by team, component, environment stage, or any other dimension. A single app can have multiple tags, and tags are purely organizational (they do not affect how an app runs).

**Relevant packages:**

- Backend: `internal/api/tags.go`, `internal/store/tags.go`
- CLI: `cmd/levelrail-cli/tags.go`, `cmd/levelrail-cli/apps_tag.go`
- Dashboard: `web/src/components/TagsControl.tsx`, `web/src/components/TagFilter.tsx`

## How tags work

A tag has an ID and a name. Tag names are unique across the control plane (you cannot create two tags with the same name) and capped at 64 characters for reasonable chip display in the dashboard.

Apps-only. Databases do not support tags today. Tags are soft labels with no enforcement or special behavior: they do not prevent deployment, restrict access, or gate anything.

## Creating and managing tags

### Via CLI

```bash
# List every tag
levelrail-cli tags list

# Create a new tag
levelrail-cli tags create --name backend

# Delete a tag (detaches it from every app it was on)
levelrail-cli tags delete tag_abc123

# List every app attached to a tag
levelrail-cli tags apps tag_abc123
```

### Via API

```bash
# Create a tag
curl -X POST https://control-plane/api/v1/tags \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"backend"}'

# List all tags
curl https://control-plane/api/v1/tags \
  -H "Authorization: Bearer $TOKEN"

# Delete a tag
curl -X DELETE https://control-plane/api/v1/tags/tag_abc123 \
  -H "Authorization: Bearer $TOKEN"

# List apps tagged with a specific tag
curl https://control-plane/api/v1/tags/tag_abc123/apps \
  -H "Authorization: Bearer $TOKEN"
```

## Attaching and detaching tags from apps

Tag attach/detach is scoped under the app resource, not under tags themselves, the same pattern [alerts](./observability.md) and feature flags use for per-app child resources.

### Via CLI

```bash
# Attach a tag (by name) to an app
# If the tag doesn't exist, it is created automatically
levelrail-cli apps tag my-app backend

# Detach a tag (by ID) from an app
levelrail-cli apps untag my-app tag_abc123

# Get the tag IDs attached to an app
levelrail-cli apps get my-app | jq '.tags'
```

### Via API

```bash
# List tags attached to an app
curl https://control-plane/api/v1/apps/my-app/tags \
  -H "Authorization: Bearer $TOKEN"

# Attach a tag by name (creates the tag if it doesn't exist)
curl -X POST https://control-plane/api/v1/apps/my-app/tags \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"backend"}'

# Detach a tag from an app
curl -X DELETE https://control-plane/api/v1/apps/my-app/tags/tag_abc123 \
  -H "Authorization: Bearer $TOKEN"
```

## Filtering by tag in the dashboard

The apps list (/apps) shows tag chips on every app row. Click a chip to filter the entire list to only apps with that tag. The filter control at the top of the page shows the active filter and a clear button.

## Use cases

**By team:**

Tag apps owned by different teams (frontend, backend, infrastructure, data) so each team can quickly filter to their responsibilities.

**By layer/component:**

Mark payment-critical services, cache tiers, API gateways, workers, and batch jobs so you can quickly find related services and track them together.

**By lifecycle stage:**

Tag production-ready services differently from experimental or staging services for visual scanning.

**By dependency relationship:**

Tag services that depend on a specific database, message queue, or external service so you know what else to restart or troubleshoot when that dependency breaks.

## API reference

| Method | Path | Ability | Purpose |
| --- | --- | --- | --- |
| `POST` | `/api/v1/tags` | `write` | Create a tag |
| `GET` | `/api/v1/tags` | `read` | List all tags |
| `DELETE` | `/api/v1/tags/{id}` | `write` | Delete a tag (detaches from all apps) |
| `GET` | `/api/v1/tags/{id}/apps` | `read` | List apps tagged with a specific tag |
| `GET` | `/api/v1/apps/{name}/tags` | `read` | List tags attached to an app |
| `POST` | `/api/v1/apps/{name}/tags` | `write` | Attach a tag to an app (creates tag if it doesn't exist) |
| `DELETE` | `/api/v1/apps/{name}/tags/{id}` | `write` | Detach a tag from an app |

## Implementation notes

**Tag create-on-attach**

When you attach a tag to an app by name (e.g., `levelrail-cli apps tag my-app backend`), if a tag with that name doesn't exist yet, the server creates it automatically before attaching it to the app. This gives the deploy-time convenience you get from app.yaml's `env: { from: ... }` resolution, scaled down to this one field.

If multiple CLI/API calls race to create the same tag name simultaneously, the server handles the race safely: whoever loses the create race re-fetches the tag that won the race and attaches that one instead.

**Deleting and orphaning**

Deleting a tag removes it from every app it was attached to. The tag itself is deleted from the store; it never leaves orphaned references.

## See also

- [Deploying apps](./deploying-apps.md) - app lifecycle and management
- [Projects, organizations, and environments](./projects-and-organizations.md) - an alternative organizational structure with shared environment variables
