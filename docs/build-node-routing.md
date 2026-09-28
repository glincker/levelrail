---
description: Which node a deploy's build runs on, how to mark a node build-only, and what to do when one node is under sustained load.
---

# Build node routing

On a single node, every build already runs where it always has: locally,
alongside the control plane and every running app. Nothing here changes
that default, and nothing on this page needs deciding until you add a
second node.

## What `accepts_build_workloads` does

Every node (including the local one, once you add a real second node) has
an `accepts_build_workloads` flag, set from that node's own detail page in
the dashboard, `PUT /api/v1/nodes/{id}/workloads`, or:

```
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL
```

This is a routing preference, not a placement rule and not a guarantee:

- It does not move apps or databases already running on that node.
- It does not stop a build from ever running on that node. If no node that
  accepts builds is currently online, a build still runs on the primary
  node rather than failing. A dedicated build node blipping offline should
  never be the reason a deploy fails.

## How a build picks its node

For each build, the control plane looks at every registered node that has
`accepts_build_workloads` set:

1. If none has it set, the build runs locally, the same as a fresh
   single-node install.
2. If one or more do, and at least one of those is online right now, the
   online one with the lexicographically smallest ID runs the build. This
   is a deterministic tie-break, not load-based scheduling: real
   scheduling (bin-packing, affinity, autoscaling) is out of scope, the
   same non-goal that applies to app and database placement.
3. If one or more have it set but none is currently online, the build
   falls back to the primary node rather than failing.

## When to mark a node build-only

Once you have more than one node, mark a secondary node's
`accepts_build_workloads` on and, if you want builds to stay off the
primary entirely, mark the primary's off. A build that runs on a node
already serving traffic competes with that traffic for CPU and disk for
the length of the build, so moving builds to a node that isn't serving
anything keeps that competition off your production containers.

## The nodes page suggestion

On a single-node install, the Nodes page can show a suggestion when that
node's CPU or disk usage has been sustained high (the same threshold the
node resource usage alert already uses, `APP_ALERT_NODE_CPU_THRESHOLD_PERCENT`
and the disk-space alert's own threshold, no separate mechanism). It
points at adding a second node, and once you have one, marking it (or the
primary) build-only per the section above. It only appears when telemetry
is configured and only for a single node; it has nothing new to say once
node routing is already something you've configured.
