---
title: "API v2: uniform resource view"
description: "The v2 API gives apps, nodes and databases one object shape, a resource version and list and watch, mapped from the v1 data on each request."
"og:url": "https://docs.levelrail.com/api-v2"
---

The v2 routes give apps, nodes and databases one object shape, a resource version and list-and-watch. They are additive: every v1 route and table keeps working and stays the source of truth. v2 objects are mapped from the v1 data on each request, nothing is stored twice.

## Object shape

```json
{
  "apiVersion": "<brand domain>/v2",
  "kind": "App",
  "metadata": {
    "name": "web",
    "namespace": "<project id>",
    "labels": {"tier": "web"},
    "annotations": {"<brand domain>/tags": "prod,eu"},
    "resourceVersion": "42"
  },
  "spec": { "...": "the v1 app body, with the same env redaction" },
  "status": {"label": "Healthy", "variant": "success"}
}
```

- Apps: `namespace` is the project, `labels` are the app labels, tags appear as one annotation. `spec` is the v1 `GET /api/v1/apps/{name}` body with sensitive env values masked exactly like v1. Secret values never appear; `secret_env` lists names only.
- Nodes: `labels` are the node labels plus `region`, `zone` and `provider`. `status.phase` is the node status.
- Databases: `spec` is the v1 database body.

`resourceVersion` on an object is the newest change recorded for it. Nodes and databases do not record changes yet, so their objects carry no version.

## Routes

| Route | Permission (same as v1) |
| --- | --- |
| `GET /api/v2/apps`, `GET /api/v2/apps/{name}` | read, filtered per app |
| `GET /api/v2/databases`, `GET /api/v2/databases/{name}` | read, filtered per database |
| `GET /api/v2/nodes`, `GET /api/v2/nodes/{name}` | root |

List parameters:

- `labelSelector`: comma separated equality terms, `k=v`, `k==v`, `k!=v`, `k` (exists), `!k` (absent). Set based terms are not supported yet.
- `limit` and `continue`: items are sorted by name; pass the returned `metadata.continue` to get the next page. Pages are read live, not from one snapshot.

The list's `metadata.resourceVersion` is the newest version at the time of the list. Watch from it to see every later change.

## Watch

`GET /api/v2/apps?watch=true&resourceVersion=N` streams every app change after `N` as server-sent events. Without `resourceVersion` the stream starts at the current version. Watch is available for apps only today.

```
event: MODIFIED
id: 43
data: {"type":"MODIFIED","object":{...}}
```

- Event types: `ADDED`, `MODIFIED`, `DELETED`, `BOOKMARK` and `ERROR`. A bookmark carries only `metadata.resourceVersion`, so an idle client can resume close to the head.
- Events are filtered per object with the caller's v1 read check, and the stream ends when the caller loses read access. A label selector filters events too.
- Watch objects are built from the change log, which never stores env values or file contents, so every env value reads as masked.
- `410 Gone` with `"reason":"Expired"` means `N` is older than the retained history. List again and watch from the list's `metadata.resourceVersion`.
- A client that falls more than one buffer behind gets a final `ERROR` event with `"reason":"TooSlow"` and the stream closes; relist and resume. Writers are never slowed by a watcher.

## Settings

| Env | Default | Meaning |
| --- | --- | --- |
| `APP_RESOURCE_CHANGES_RETAIN` | 10000 | Changes kept by count |
| `APP_RESOURCE_CHANGES_MAX_AGE` | 168h | Changes kept by age |
| `APP_RESOURCE_CHANGES_COMPACT_EVERY` | 100 | Compact once every this many changes |
| `APP_WATCH_BUFFER` | 256 | Events read per poll and the lag allowed before a client is dropped |
| `APP_WATCH_MAX_STREAMS` | 64 | Open watch streams per control plane; more get 429 |
| `APP_WATCH_BOOKMARK_INTERVAL` | 30s | Bookmark cadence |
| `APP_WATCH_POLL_INTERVAL` | 1s | How often a stream reads the change log |
| `APP_WATCH_WRITE_TIMEOUT` | 10s | Per event write deadline |

Compaction always keeps the newest change of every live object, so object versions survive it.

## Go client

`internal/apiclient` has `ListResources`, `GetResource`, `WatchResources` and `IsResourceExpired` for these routes.
