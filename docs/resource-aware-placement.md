---
title: "Resource-aware placement"
tag: "Beta"
description: How new apps and databases are placed on nodes with room for them, how to read the "why here" explanation, and how to set the overcommit policy.
---

When you create an app or database without choosing a node, or when a move or drain has no explicit target, the control plane picks a node that can actually hold it. It looks at each node's CPU, memory and disk, filters out nodes that cannot fit the workload, and spreads onto the node that keeps the most headroom. Every decision is recorded so you can see why a workload landed where it did.

This is a fit check and a spread, not a packing solver. Bin-packing and affinity rules are still non-goals.

## What a node offers and what a workload asks for

- **Allocatable** is what the node reports about itself: CPU cores, total memory and disk. The local node is read directly. A remote node's agent reports it every 30 seconds.
- **Requested** is the sum of `resources.cpu` and `resources.memory` (times replicas) of every app placed on the node. Databases carry no declared request today, so they count toward load but not toward requested totals.
- **Disk** is measured, not requested: a node is refused when placing would leave less than the minimum free share of its disk.

## How a node is chosen

1. Nodes that are cordoned, offline, excluded (the node being drained) or missing a selector label are filtered out, as before.
2. Nodes that cannot fit the workload's CPU or memory request are filtered out, with the shortfall as the reason.
3. Among the rest, the node with the most free share of its tightest resource after the placement wins. Ties go to the node with fewer workloads, then to the lowest node id, so the same inputs always give the same answer.
4. When the control plane's own node serves ingress for a new app with domains, it is preferred if it fits.

An explicit `node_id` or move target is always honored. If the pinned node would not have fit, the decision records a warning instead of refusing.

## Unknown capacity

A node that has not reported a figure (an agent that has not connected yet, or an older agent) is treated as **unknown**, never as zero (nothing fits) and never as unlimited (everything fits).

- A dimension the workload does not ask for is ignored.
- A node whose capacity cannot be verified for a requested dimension stays eligible, but ranks after every verified node, and the decision carries a warning.
- Set `APP_PLACEMENT_REJECT_UNKNOWN_CAPACITY=true` to filter such nodes out instead.
- Disk is always checked, so a node with no disk reading is unverified until its first report.

## When nothing fits

A create with no `node_id` fails with `409` and lists each node's shortfall, for example `node-a: insufficient capacity: memory: needs 2.0 GiB, 1.0 GiB free of 1.0 GiB allocatable (short by 1.0 GiB)`. Free capacity, add a node, raise the overcommit setting, or pass `node_id` to place it on a node anyway. A drain reports the same per-resource error and leaves that resource where it is.

If no node is registered, or every node is cordoned or offline, the create still falls back to the local node as before.

## The overcommit policy

Operator settings on the control plane, read at start:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PLACEMENT_CPU_OVERCOMMIT_PERCENT` | `100` | Requested CPU may total this share of the node's cores. |
| `APP_PLACEMENT_MEMORY_OVERCOMMIT_PERCENT` | `100` | Requested memory may total this share of the node's memory. |
| `APP_PLACEMENT_MIN_FREE_DISK_PERCENT` | `10` | Share of disk that must stay free after a placement. |
| `APP_PLACEMENT_REJECT_UNKNOWN_CAPACITY` | `false` | Filter out nodes whose capacity is unknown. |

The default never overcommits. Values above `100` allow it; CPU is compressible and usually safe to raise, memory is not. Invalid values are logged and the safe default is used.

## Seeing why

**Dashboard.** The node page has a **Capacity** card with requested versus allocatable bars. The app and database **Placement** card shows the recorded decision (the nodes considered, which were refused and why, the winner) and a fresh evaluation of every node right now.

**CLI.**

```bash
levelrail-cli apps placement web            # why "web" runs where it does
levelrail-cli apps placement db --database
levelrail-cli nodes allocations             # requested vs allocatable per node, plus the policy
```

**API.**

- `GET /api/v1/apps/{name}/placement/decision` and `GET /api/v1/databases/{name}/placement/decision` return `recorded` (trigger, time, and the decision) and `preview`.
- `GET /api/v1/nodes/allocations` returns every node's dimensions and the policy in force.

Only the latest decision per resource is kept. A workload placed before decisions were recorded shows none until it is next placed, moved or drained.
