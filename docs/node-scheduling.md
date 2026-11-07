---
title: "Node scheduling: taints, tolerations, affinity and score mode"
tag: "Beta"
description: Keep workloads off a node with taints, let chosen apps onto it with tolerations, place apps near or away from each other with affinity, and pick spread or bin-pack scoring live.
"og:url": "https://docs.levelrail.com/node-scheduling"
---

<Warning>
Not yet verified on a real multi-node host. The rules below are covered by unit and API tests on a single control plane. Treat this page as a preview until a two-node run is recorded here.
</Warning>

Nothing on this page changes a one-node install. With no taints, no tolerations, no affinity rules and the default `spread` score mode, every placement is decided exactly as described in [Resource-aware placement](/resource-aware-placement).

## How a node is chosen

For each new app or database without an explicit `node_id`, and for each move or drain without a target, the control plane:

1. Drops nodes that are offline, cordoned, missing a `node_selector` label or too small for the workload's requests.
2. Drops nodes with a `NoSchedule` or `NoExecute` taint the workload does not tolerate.
3. Drops nodes that fail a required affinity term (`node_required`, or a `together` or `apart` term with weight `0`).
4. Scores the rest: the score mode picks the base order, preferred affinity terms add or subtract weight, and an untolerated `PreferNoSchedule` taint ranks the node lower.

An explicit `node_id` always overrides this choice.

## Taints: reserve a node

```bash
# keep everything off this node unless it tolerates dedicated=gpu
levelrail-cli nodes taint <node-id> dedicated=gpu:NoSchedule
# a key with no value
levelrail-cli nodes taint <node-id> maintenance:PreferNoSchedule
# show the taints, then remove them all
levelrail-cli nodes taint <node-id>
levelrail-cli nodes taint <node-id> --clear
```

- A taint is `key[=value]:Effect`. Passing taints replaces the node's whole set.
- `NoSchedule` and `NoExecute` keep new placements off the node. `PreferNoSchedule` only ranks it lower.
- Workloads already running on the node stay where they are. Use `levelrail-cli nodes drain <node-id>` to move them.
- API: `GET` and `PUT /api/v1/nodes/{id}/taints` with body `{"taints":[{"key":"dedicated","value":"gpu","effect":"NoSchedule"}]}`. Root only.

## Tolerations: let an app onto a tainted node

```bash
# tolerate one exact taint
levelrail-cli nodes placement app web --toleration dedicated=gpu:NoSchedule
# tolerate any value of a key, for any effect
levelrail-cli nodes placement app web --toleration-exists dedicated
# remove the app's tolerations and affinity
levelrail-cli nodes placement app web --clear-placement-rules
```

A toleration allows a node; it does not attract the app to it. Pair it with a `node_selector` label or a `node_required` term to pin the app there.

## Affinity: near, away, or on a kind of node

```yaml
# affinity.yaml
node_required:            # every term must match the node's labels
  - {key: disk, op: In, values: [ssd]}
node_preferred:           # soft, weight 1 to 100
  - weight: 50
    term: {key: zone, op: In, values: [eu-1]}
together:                 # prefer nodes already running matching apps
  - match: [{key: name, op: In, values: [cache]}]
    weight: 20
apart:                    # weight 0 means required: never share a zone with db
  - match: [{key: name, op: In, values: [db]}]
    topology: zone
```

```bash
levelrail-cli nodes labels <node-id> disk=ssd zone=eu-1
levelrail-cli nodes placement app web --affinity-file affinity.yaml
```

- Operators are `In`, `NotIn`, `Exists` and `DoesNotExist`.
- Apps are matched by their labels and by `name=<app name>`.
- `topology` is `node` (the default) or a node label key such as `zone` or `region`.
- The term count per app is capped by `APP_PLACEMENT_AFFINITY_MAX_TERMS` (8 unless set). Invalid rules return `400` with the reason.

The same fields go under a service in `app.yaml`:

```yaml
services:
  web:
    tolerations:
      - {key: dedicated, value: gpu, effect: NoSchedule}
    affinity:
      node_required:
        - {key: disk, op: In, values: [ssd]}
```

API: `PUT /api/v1/apps/{name}/placement` takes `node_selector`, `tolerations` and `affinity` (databases take `node_selector`). Leaving `tolerations` or `affinity` out of the body keeps what is stored.

## Score mode: spread or bin-pack

```bash
levelrail-cli settings runtime get placement.score_mode
levelrail-cli settings runtime set placement.score_mode binpack
levelrail-cli settings runtime set placement.score_mode spread
```

- `spread` (default) picks the node that keeps the most headroom.
- `binpack` fills the fullest node that still fits, which leaves whole nodes free.
- The setting is read on every placement and needs no restart. `APP_PLACEMENT_SCORE_MODE` only seeds the default. The dashboard has it under Settings, Runtime.

## See why a node was picked or skipped

```bash
levelrail-cli nodes placement app web
```

The output lists the eligible nodes, every rejected node with its reason (taint, affinity, selector, capacity) and the stored tolerations. `GET /api/v1/apps/{name}/placement/decision` returns the recorded placement decision for the app.

## Not built yet

- One instance per matching node (`workload: per-node` in `app.yaml`) is accepted by validation but not scheduled yet.
- `NoExecute` does not evict running workloads by itself. A drain does.
