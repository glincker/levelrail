---
title: "Build node routing"
description: Which node a deploy's build runs on, how to mark a node build-only, and what to do when a single node is under sustained load.
"og:url": "https://docs.levelrail.com/build-node-routing"
---

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

For each build, the control plane looks at every registered node with `accepts_build_workloads` set. A node is skipped when it is offline, draining, or has not been seen within `build.runner_stale_after`. The strategy in `build.runner_strategy` then picks among the rest:

- `first`: the usable node with the lexicographically smallest ID. A fixed tie-break, and the default.
- `least_loaded`: the node with the most free build slots, counting queued builds and a warning pressure level against it.
- `sticky_per_app`: the node that last built this app, so its layer cache is reused, while it still has a free slot.

With `least_loaded` and `sticky_per_app`, a build for a given architecture only goes to nodes of that architecture, and with `build.remote_pressure_hold` on, nodes under critical host pressure are skipped. Required labels apply to every strategy. If nothing qualifies, the build runs on the control plane instead of failing, so a build node going offline never fails a deploy. Levelrail does not do bin-packing, affinity or autoscaling.

When a build runs on another node, the built image is streamed back and loaded into the control plane's own image store, so the rest of the deploy works the same as for a local build.

The strategy and both timings are runtime settings: change them under Settings, Security and runtime (deploys category). They apply live.

## The Runners panel

The Deployments page lists every runner as a card, polled while the page is open:

- The status badge reads Ready, Busy, Under pressure, Draining or Offline.
- The capacity bar is slots in use over total slots, with the number of builds waiting for a slot.
- Each card also shows load, architecture, CPU cores, labels and the last build (app, outcome and age). The control plane card adds the build cache size.
- The active strategy is shown above the cards, with a link to the setting.

With only the control plane listed, the panel explains how to add a build node and links to the Nodes page.

## Per-app preference

An app can prefer a build node. On the app's Deploy settings page, under Build, pick a node, a set of labels (`key` or `key=value`, comma separated), or both. A chosen node is a soft preference. Turn on pin to make it strict: if that node cannot build, the build runs on the control plane, never on another node. Clear removes the preference and the app follows the global strategy again. Changing it needs deploy access on the app, because it decides where the app's source and secrets are read.

```bash
curl -X PUT $LEVELRAIL_URL/api/v1/apps/web/build-runner \
  -H "Authorization: Bearer $LEVELRAIL_TOKEN" \
  -d '{"node_id":"<build-node-id>","pinned":false,"labels":["arch=arm64"]}'
```

## Where a build ran

The run page header shows the runner a build used and how long it waited in the queue. The same fields, `runner_name`, `runner_reason` and `queue_wait_ms`, are on each row of `GET /api/v1/builds` and `GET /api/v1/builds/{id}`. Deploys without a build, such as image-only deploys, show nothing.

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

A node provisioned with `nodes provision --role build` or `nodes ssh-provision --role build` is set up as a build node when it enrolls. See [Node provisioning](/node-provisioning).

## The Nodes page suggestion

On a single-node install, the Nodes page can suggest adding a second node when that node's CPU or disk use has been high for a sustained period. It reuses the existing node resource usage alert (`APP_ALERT_NODE_CPU_THRESHOLD_PERCENT`) and the disk space alert, with no separate threshold. It appears only when telemetry is configured and only while there is exactly one node.

## See also

- [Multi-node](/multi-node): enrolling nodes and managing placement.
- [Observability](/observability): the node alerts behind the suggestion.

To cap how long builds run and how many run at once on any node, see [Build performance](/build-performance).
