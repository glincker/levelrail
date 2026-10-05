---
description: Which node a deploy's build runs on, how to mark a node build-only, and what to do when a single node is under sustained load.
---

# Build node routing

On a single node, every build runs locally, alongside the control plane and every running app. Nothing on this page needs deciding until you add a second node.

## The `accepts_build_workloads` flag

Every node has an `accepts_build_workloads` flag. It is a routing preference, not a placement rule and not a guarantee. Set it from the node's detail page in the dashboard, with the CLI, or with `PUT /api/v1/nodes/{id}/workloads`:

```bash
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL
```

Both flags are required on every call, because the command replaces both values. A new node accepts app workloads but not build workloads until you enable it.

What the flag does not do:

- It does not move apps or databases already running on the node.
- It does not stop a build from running on a node without it. If no node that accepts builds is online, the build runs on the primary node instead of failing, so a build node going offline never fails a deploy.

## How a build picks its node

For each build, the control plane looks at every registered node with `accepts_build_workloads` set:

1. If none has it set, the build runs locally, as on a fresh single-node install.
2. If at least one is online, the online one with the lexicographically smallest ID runs the build. This is a fixed tie-break, not load-based scheduling. Levelrail does not do bin-packing, affinity or autoscaling.
3. If some have it set but none is online, the build falls back to the primary node.

When a build runs on another node, the built image is streamed back and loaded into the control plane's own image store, so the rest of the deploy works the same as for a local build.

## Mark a node build-only

A build on a node that already serves traffic competes with that traffic for CPU and disk for as long as the build runs. To keep builds off your production containers:

<Steps>
<Step title="Enable builds on a secondary node">

```bash
levelrail-cli nodes workloads <build-node-id> --accepts-app=false --accepts-build=true
```

Use `--accepts-app=true` instead if the node should also run apps.

</Step>
<Step title="Optionally disable builds on the primary">

```bash
levelrail-cli nodes workloads <primary-node-id> --accepts-app=true --accepts-build=false
```

</Step>
</Steps>

A node created with `nodes provision --role build` or `nodes ssh-provision --role build` is set up as a build node when it enrolls. See [Node provisioning](node-provisioning.md).

## The Nodes page suggestion

On a single-node install, the Nodes page can suggest adding a second node when that node's CPU or disk use has been high for a sustained period. It reuses the existing node resource usage alert (`APP_ALERT_NODE_CPU_THRESHOLD_PERCENT`) and the disk space alert, with no separate threshold. It appears only when telemetry is configured and only while there is exactly one node.

## See also

- [Multi-node](multi-node.md): enrolling nodes and managing placement.
- [Observability](observability.md): the node alerts behind the suggestion.
