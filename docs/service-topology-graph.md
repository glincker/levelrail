---
title: "Project topology graph"
description: Read a project's topology diagram, what each node and edge represents, and exactly which stored field each edge is derived from.
---

The **Topology** page (`/projects/$id/topology`, linked from a project's own detail page) draws a project's apps, databases, and shared volumes as a diagram, with lines representing every relationship between them. It is read-only and requires no configuration: every fact on the page already exists in the project's desired state, just arranged as a graph instead of a list.

[Network topology](/network-topology) groups everything on the mesh by node and draws only app-to-database connections. This page is scoped to one project, groups by resource kind, and draws every edge kind the project's configuration carries: database bindings, service dependencies, shared volumes, and egress allow-rules.

## What you see

An interactive canvas. Drag the background to pan, scroll to pan or hold Ctrl or Cmd while scrolling to zoom, and drag any box to rearrange it. Columns start as **Domains**, **Apps**, **Databases** and **Volumes**. Each app and database box shows the same status dot used everywhere else in the dashboard (healthy, deploying, failed, no status), refreshed every few seconds, so a deploy in flight is visible without leaving the page.

Click a box to open a side panel with its connections and quick links (overview, logs, deploys and rollback, domains, metrics). Positions are remembered per project in this browser; **Reset layout** returns to the default arrangement. Large projects stay responsive because boxes and lines outside the visible area are not drawn until you pan to them.

Keyboard: Tab to the canvas, arrow keys pan, plus and minus zoom. Tab to a box, then arrow keys nudge it (hold Shift for bigger steps), Enter opens its panel, Escape closes it.

## What each edge means

Every edge is derived from one specific field on the project's apps and databases. None are inferred or illustrative: an edge only appears when the underlying configuration actually declares the relationship.

| Edge | Drawn from | Example |
| --- | --- | --- |
| Database binding | An app's `env: { FOO: { from: "db.field" } }` binding ([`EnvVar` reference](/app-spec-reference#envvar-an-entry-under-env)), or a direct database attachment made via `levelrail-cli apps connect` ([Connecting apps to databases](/connecting-apps-to-databases)) | `web` reads `DATABASE_URL` from database `main` |
| Depends on | An app's `dependsOn:` list, resolved the same way the reconciler resolves it before starting a container: the dependency must be a real sibling service in the same multi-service app | `web` depends on `worker` |
| Volume attachment | A named Docker volume an app mounts (`PUT /apps/{name}/volumes`, or `app.yaml`'s `volumes:` block). Two apps that happen to mount the same volume name render as one shared volume box, not two | `web` mounts `app-web-uploads` |
| Routing | A domain listed in an app's `domains:`. The domain box points at the app that serves it | `shop.example.com` routes to `web` |
| Load balances | An app with a [load balancer](/api-reference) configured. The load balancer box points at the app it balances | `web` has a balancer in front of it |
| Runs on | The fleet node an app or database is placed on. The app or database box points at the node box | `main` runs on `edge-1` |
| Egress allow-rule | An entry in an app's [egress policy](/api-reference) allowlist, but only when that entry's host matches a domain another app in the same project actually owns. An allow-listed third-party host (the common case, e.g. `api.stripe.com`) has nothing to point at in this project and is left off the diagram rather than drawn as a guess | `web` is allowed to reach `admin` over its `admin.example.com` domain |

[Network shares](/network-shares) (`/api/v1/network-shares`) are left off this diagram: nothing in a project's stored configuration links a share to an app, so an edge would be a guess.

## CLI and API

```bash
levelrail-cli apps projects topology <project-id>
```

This prints the graph as nodes and edges (kinds `app`, `database`, `volume`, `domain`, `load_balancer`, `node`; edge kinds `database_binding`, `depends_on`, `volume_attachment`, `egress_allow`, `routing`, `load_balances`, `placed_on`); add `--json` for the raw response, the same JSON the canvas draws. The API is `GET /api/v1/projects/{id}/topology` (`read` ability). Whole-mesh data comes from `levelrail-cli nodes topology`, covered on [Network topology](/network-topology).

## Multi-service apps

A multi-service app (created from a `compose:` build or `POST /apps/{name}/compose`) shows one box per member service, not one collapsed box for the whole app: `dependsOn` and database bindings are declared per service, so collapsing them would hide exactly the relationships this page exists to show.

## See also

- [Network topology](/network-topology) - the whole-mesh, node-grouped view with live mesh reachability
- [Projects and organizations](/projects-and-organizations) - what a project is and how apps/databases get filed under one
- [Connecting apps to databases](/connecting-apps-to-databases) - how a database binding is created in the first place
- [App spec reference](/app-spec-reference) - `dependsOn:`, `volumes:`, and `env: { from: ... }` syntax
- [API reference](/api-reference) - `GET /api/v1/projects/{id}/topology`'s full request/response shape
