---
title: "Add a node anywhere"
tag: "Beta"
sidebarTitle: "Add a node anywhere"
description: One guided flow to add an EC2 instance, a Google Cloud VM, a VPS or a home server as a node, with provider, region and zone auto-detect, live progress, preflight checks with fixes and failure-domain aware placement.
"og:url": "https://docs.levelrail.com/add-node-anywhere"
---

Adding a server on any cloud, a cheap VPS or a machine at home is the same flow: pick where it runs, run one command on it, watch it connect, and read the preflight result. This page covers that flow and what the control plane does with the provider, region and zone it learns.

<Warning>
This feature is beta. It is tested with fake metadata servers and a real local Docker daemon. Nothing here has been verified against the real AWS, Google Cloud, Azure, Hetzner or DigitalOcean metadata services, or across a real WAN mesh. See [What is verified](#what-is-verified).
</Warning>

## The flow

<Steps>
<Step title="Choose where it runs">

In the dashboard open **Nodes**, then **Add node**, then **I already have a server**. Leave **Where does it run?** on **Auto-detect** to let the node read its own provider, region and zone. Pick a provider and type a region and zone instead for a home server or a provider without a metadata service.

</Step>
<Step title="Run one command">

The dialog shows a Docker command and an agent binary command. Both carry a single-use join token that expires after 15 minutes by default (`APP_NODE_JOIN_TOKEN_TTL` changes it, one minute to 24 hours). Give the node a name to follow it live.

</Step>
<Step title="Watch it connect">

With a name set, the dialog shows four stages: join token created, agent contacted the control plane, connected over mTLS, preflight checks. It updates by itself.

</Step>
<Step title="Read the preflight">

Every check shows its status. Anything that is not a pass shows a concrete fix, with the shell command collapsed under **Fix via CLI**. **Re-check** reads the latest report; the agent resends its facts every minute, so a fix you apply on the node shows up without a restart.

</Step>
</Steps>

From the CLI the same flow is:

```bash
levelrail-cli nodes add --name edge-1 --preflight
levelrail-cli nodes add --name home-1 --provider home --region basement --zone rack-1 --preflight
levelrail-cli nodes preflight edge-1
```

`--preflight` prints the join command, waits for the node, then prints each check and exits non-zero when one failed. `nodes preflight <id-or-name>` shows the last result; add `--wait` to wait for it.

## Provider, region and zone detection

On every session the agent asks the metadata service of the cloud it runs on. Providers are probed in a fixed order, so a host that answers on more than one gives a stable result.

| Provider | Endpoint the agent reads | Region | Zone |
| --- | --- | --- | --- |
| AWS | IMDSv2: a token request, then `placement/availability-zone` and `placement/region` | region | availability zone |
| Google Cloud | `computeMetadata/v1/instance/zone` with `Metadata-Flavor: Google` | derived from the zone | zone |
| Azure | `metadata/instance/compute` with `Metadata: true` | location | location plus the zone number, when one is assigned |
| Hetzner | `hetzner/v1/metadata/region` and `availability-zone` | region | availability zone |
| DigitalOcean | `metadata/v1/region` | region | none (DigitalOcean has no zones) |

Values are made valid as node labels (letters, digits, `.`, `_`, `-`, at most 63 characters). The control plane applies a detected value only where the node has none: a label you set by hand always wins, and a detected value never overwrites it.

### When there is no metadata service

A home server or an unsupported provider reports nothing, and the node simply has no labels until you set them. Three ways to set them:

- Fill in the provider, region and zone in the dialog, or pass `--provider`, `--region` and `--zone` to `nodes add`. The join command then carries `APP_NODE_PROVIDER`, `APP_NODE_REGION` and `APP_NODE_ZONE`.
- Set those variables on the agent yourself. They override what metadata reports, field by field.
- Label the node afterwards on its page, or with `levelrail-cli nodes labels`.

Set `APP_AGENT_METADATA_DISABLE=1` on the agent to skip the metadata probe entirely, and `APP_AGENT_METADATA_TIMEOUT` (default `1s`) to change how long each provider is given.

## Preflight checks

The agent measures facts and the control plane judges them, so thresholds and fix text live in one place.

| Check | Fails when | Fix shown |
| --- | --- | --- |
| Control plane reachable | never fails here: the report only arrives over a working mTLS session | none |
| Docker | the daemon does not answer on the local socket | install Docker and let the agent read the socket |
| Architecture | the node is not Linux on a supported CPU (default `amd64`, `arm64`) | use a supported machine |
| Disk space | free space on the agent data directory is under the minimum | free space or grow the disk |
| Clock skew | the node clock differs from the control plane by more than the limit | enable time synchronization |
| Kernel WireGuard | warns when the module is not loaded, so the mesh would use the slower userspace path | load the module |
| Mesh port | warns when UDP 51820 is already in use on the node | stop what holds it |
| Firewall and security group | never passes: it is reported as unverified | the exact inbound rule to open, with the provider's documentation link |

Thresholds are read on the control plane:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_NODE_PREFLIGHT_MIN_DISK_FREE_MB` | `5120` | minimum free disk; a warning shows below twice this |
| `APP_NODE_PREFLIGHT_MAX_CLOCK_SKEW` | `30s` | clock skew that fails; a warning shows above half of it |
| `APP_NODE_PREFLIGHT_ARCHITECTURES` | `amd64,arm64` | supported CPU architectures |
| `APP_AGENT_ONBOARDING_INTERVAL` | `1m` | how often the agent resends its facts (set on the agent) |

<Note>
A node only dials out, so the agent cannot prove that other nodes can reach its mesh port from outside. The firewall check says so and shows the rule to open. Once the mesh has two nodes, [node pair diagnostics](/multi-node#pair-diagnostics) report whether the handshake actually completes. The clock check includes network latency, so it is accurate to about a second.
</Note>

## Failure-domain aware placement

Nodes carry `provider`, `region` and `zone`. When any eligible node has one, placement spreads replicas of the same workload across them: the zone holding the fewest replicas wins, then the region, then the provider, then the zone carrying the fewest workloads, then the usual headroom order. The choice is deterministic: the same fleet and the same request always give the same answer, and offline or cordoned nodes never win for the sake of diversity.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PLACEMENT_REGION_SPREAD_WEIGHT` | `1` | weight of region spread; `0` turns it off |
| `APP_PLACEMENT_PROVIDER_SPREAD_WEIGHT` | `1` | weight of provider spread; `0` turns it off |

### Warnings on apps

All replicas of one app run on one node today, so an app with two or more replicas always shares fate. The app's placement card and `GET /api/v1/apps/{name}/placement` list a warning when the app's node sits in a labeled zone and another zone has an eligible node (a warning), or the same for providers (informational). Fixing it means moving the app, or running separate apps per zone with placement selectors. Spreading the replicas of one app across nodes is not built.

### Capacity overview

The **Nodes** page shows a card with reported CPU, memory and free disk per provider, region and zone, with the number of nodes and workloads in each. Capacity sums only what nodes reported; a node that reported nothing is counted as unverified, never as zero.

```bash
levelrail-cli nodes domains
```

## API, CLI and MCP

| Surface | How |
| --- | --- |
| API | `GET /api/v1/nodes/{id}/onboarding`, `GET /api/v1/nodes/domains`, and the optional `provider`, `region` and `zone` fields of `POST /api/v1/nodes/join-tokens` |
| CLI | `nodes add --provider --region --zone --preflight`, `nodes preflight`, `nodes domains` |
| MCP | the read tool `list_zones`: pass `node_id` for a node's preflight with fixes, or `capacity` for the per-domain totals |

## What is verified

- Metadata parsing is tested against fake servers that mimic each provider's request and response shape, including AWS token handling and the Google `Metadata-Flavor` echo.
- Preflight judging, placement spread and the API are covered by unit tests.
- The two-Docker-daemon join test reports preflight facts from two real agents against two real daemons and checks that the labels reach the node rows. It runs only with `LEVELRAIL_LIVE_MULTINODE=1`.

Not verified: any real cloud metadata endpoint (response shapes come from provider documentation), a real WAN mesh, inbound reachability of the mesh port, and the clock check across real network latency. Treat the whole flow as beta until it has run against real instances.

## Related pages

<CardGroup cols={2}>
<Card title="Multi-node" href="/multi-node">
Enrollment, the mesh, drains and the failure policy.
</Card>
<Card title="Resource-aware placement" href="/resource-aware-placement">
How the scheduler fits and ranks nodes.
</Card>
<Card title="Node provisioning" href="/node-provisioning">
Create a server at a cloud provider for you.
</Card>
</CardGroup>
