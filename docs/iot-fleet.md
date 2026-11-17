---
title: "Device fleets"
sidebarTitle: "Device fleets"
description: "Label a fleet of small devices, list them as a group, and roll an image out to the group in batches."
"og:url": "https://docs.levelrail.com/iot-fleet"
---

A fleet is a set of nodes that share a label. There is no separate group object: a device group is a node label selector, so it exists as long as a node carries the label. Nothing on this page has been run on real devices by the Levelrail team. The commands and the group and batch logic are covered by a unit test; real hardware is marked as not verified.

## Label devices and list groups

```bash
levelrail-cli nodes labels <node-id> fleet=greenhouse site=north
levelrail-cli nodes groups list
levelrail-cli nodes groups show fleet=greenhouse
```

`groups list` prints every `key=value` pair in use with the nodes that carry it. `groups show` takes one pair or several joined by commas (`fleet=greenhouse,site=north`) and lists the nodes that match all of them. `nodes labels` replaces the node's whole label set, so pass every label you want to keep.

## Roll out to a group

Apps are bound to one node, so a fleet runs one app per device, named `<app>` or `<app>-<suffix>` (for example `sensor-pi1`, `sensor-pi2`).

```bash
levelrail-cli apps rollout sensor --image registry.example.com/org/sensor:2.4 --group fleet=greenhouse --max-unavailable 2 --dry-run
levelrail-cli apps rollout sensor --image registry.example.com/org/sensor:2.4 --group fleet=greenhouse --max-unavailable 2
```

The rollout finds the apps named `sensor` or `sensor-*` whose node matches the selector, orders them by node name, and splits them into batches of `--max-unavailable` (default 1). Each app gets the same deploy call as `apps deploy`. It waits until every app in the batch serves the image before starting the next batch. The wait per app is `LEVELRAIL_ROLLOUT_BATCH_TIMEOUT` (default `5m`). A failed or timed out app stops the rollout, and the remaining apps are reported as `skipped`. The output has one line per node with its batch number and status: `ok`, `queued`, `failed`, `skipped`, or `planned` with `--dry-run`.

The group selection and batch ordering are covered by `TestRolloutGroupSelectionAndBatchOrder` in `cmd/levelrail-cli/apps_rollout_test.go`. The wait and halt logic is not covered by a test.

## When a device is offline

What the code does, from the source:

- A change to an app on an offline node is refused with `409 node_offline` unless it carries `queue_while_offline`. The rollout sends that flag, so an offline device gets a `pending` deploy attempt and the rollout reports it as `queued` without waiting. See [Multi-node](/multi-node#what-an-offline-node-means-for-its-apps). The queueing path is covered by `TestOfflineNodeDeployQueueOverrideRecordsPending` in `internal/api/node_offline_guard_test.go`.
- When a session to the control plane fails, the agent reconnects with exponential backoff from 1 second to 30 seconds with up to 50 percent jitter, and the backoff restarts after a session that lasted a minute. This is covered by `TestNextReconnect` in `cmd/levelrail-agent/reconnect_test.go`.

Not verified: that a queued deploy is applied on a real device after it returns, and how long that takes. A device that never returns keeps the queued attempt as `pending`, and the rollout does not retry it.

## Android with Termux (experimental)

**EXPERIMENTAL and UNVERIFIED on a real device.** The agent ships for `linux/arm64`. In principle a 64-bit Android phone running Termux with a glibc or proot Linux environment could run that binary, since it looks like an arm64 Linux userland. Nobody has tried it, and Levelrail does not build or document an Android package.

Open questions that are not answered here: whether Docker's Engine API is reachable in such an environment (the agent drives Docker, and Android normally has no Docker daemon), whether the kernel allows the namespaces containers need, and whether Android keeps the process alive in the background. Treat Android as a possible future target, not a supported one.
