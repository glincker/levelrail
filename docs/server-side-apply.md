---
title: Server-side apply
description: Merge partial manifests on the server with per-field ownership, conflict detection, version preconditions and dry runs.
---

# Server-side apply

Server-side apply lets several tools (CI, an operator, the dashboard, a person running `edit`) each own part of one resource without overwriting each other. Each sends only the fields it cares about. The server merges them, remembers who set which field, and refuses a change to a field someone else owns.

Supported kinds: apps and databases. Other kinds answer `405`. Stack resources keep changing through their stack apply, and an app or database a stack owns is refused with `409`.

## API

```
PATCH /api/v2/apps/{name}?fieldManager=ci
PATCH /api/v2/databases/{name}?fieldManager=ci
Content-Type: application/apply-patch+yaml   (or application/apply-patch+json, application/json)
```

The body is a partial resource document in the same shape `export` writes:

```yaml
kind: App
metadata:
  name: web
  resourceVersion: "42"   # optional precondition
spec:
  service:
    port: 8080
```

Query parameters:

| Parameter | Meaning |
| --- | --- |
| `fieldManager` | Required. Who owns the fields this request sends. |
| `force=true` | Take over fields another manager owns instead of failing. |
| `dryRun=All` | Merge, validate and plan, but write nothing. The response holds the merged spec and the plan. |
| `resourceVersion` | Precondition, same as `metadata.resourceVersion`: the request fails with `409` unless the object is still at this version. |

The route uses the same per-object authorization as the v1 write routes (write ability plus any IAM policy on `app:NAME` or `database:NAME`).

## Merge rules

- Fields are tracked at leaf level, as JSON pointers such as `/spec/service/port`. Lists are one field.
- A field sent with the same value another manager set is shared by both.
- A field sent with a different value than another manager set is a conflict. The `409` body lists each `path` and `manager`. Pass `force=true` to take it over.
- A field a manager set before and no longer sends is removed, unless another manager still owns it.
- A field sent as `null` is removed.
- Env values you cannot read are kept as they are when you do not send them.

Ownership is returned in `metadata.managedFields` by the PATCH response and by `GET /api/v2/{kind}/{name}`. Applied changes appear on the watch stream as `MODIFIED` events.

## CLI

```
levelrail-cli apply -f web.yaml --server-side [--field-manager NAME] [--force-conflicts] [--dry-run=server]
```

The field manager defaults to the command name. `edit app NAME` uses server-side apply too: it sends only the fields you changed, forces ownership of them, and carries the version you opened as a precondition, so a concurrent change makes the save fail instead of being overwritten.

## Limits

- One server-side apply runs at a time per control plane, so the version check and the write happen together. A write through a v1 route in between can still land; the precondition catches it on the next apply.
- Database applies read the full export to find the database, which is slower on large installs.
