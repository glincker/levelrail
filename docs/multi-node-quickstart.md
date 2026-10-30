---
title: "Multi-node quickstart"
icon: "/assets/icons/hard-drives.svg"
tag: "Beta"
sidebarTitle: "Add a second server"
description: The short path to a second server, what has been verified on real infrastructure, and what to check before relying on the WireGuard mesh across hosts.
"og:image": "https://docs.levelrail.com/public/og/multi-node-quickstart.png"
"twitter:image": "https://docs.levelrail.com/public/og/multi-node-quickstart.png"
"og:url": "https://docs.levelrail.com/multi-node-quickstart"
---

This page is the short path to a second node. Each step links to the page that holds the detail, so nothing is repeated here. It also says plainly what has and has not been verified, per [Feature status](/feature-status).

<Steps>
<Step title="Get a second machine">

Pick one of two paths:

- Bring your own server: any Linux box with Docker, systemd and outbound network access works. Use [Enrolling over SSH](/multi-node#enrolling-over-ssh-instead-of-by-hand) to have the control plane set it up, or [enroll it by hand](/multi-node#enrolling-a-second-node).
- Create one at a cloud provider: Hetzner, DigitalOcean, AWS, Azure and GCP are supported. See the [multi-cloud provisioning quickstart](/multi-cloud-provisioning) for the command sequence and [Node provisioning](/node-provisioning) for credentials and cost.

</Step>
<Step title="Set the advertise host before enrolling">

Set `APP_AGENT_ADVERTISE_HOST` on the control plane to the host or IP the remote agent will dial (an IP address when the mesh is on). It defaults to `127.0.0.1`. With a mismatch, enrollment succeeds but the node's session then fails TLS hostname verification on every attempt, and the node stays `pending`. Details are in [multi-node](/multi-node#enrolling-a-second-node).

</Step>
<Step title="Enroll and confirm">

Mint a join token, run the agent with it, then check that the node reaches `online`:

```bash
levelrail-cli nodes join-token
levelrail-cli nodes list
```

A node left `pending` after its token was spent shows as "Never connected". Delete it and enroll again with a new token.

</Step>
<Step title="Place something on it">

Pick the node in an app's create form, run `levelrail-cli apps set-node <name> <node-id>`, or let [auto-placement](/multi-node#simple-spread-placement-auto-placement) choose.

</Step>
</Steps>

## What has been verified

The join flow was run with two real agent processes, each with its own Docker daemon, against a real control plane binary. Enrollment, cordon, drain with real container relocation, and explicit placement pinning all behaved as documented.

That run did not cover:

- A real WAN or second physical host. Both daemons ran on one machine, so cross-datacenter latency and NAT traversal are untested.
- The WireGuard mesh between two real hosts. The mesh code is tested against fakes and a single host with a real TUN device. If you turn it on across hosts, open inbound UDP 51820 on the control plane and check `levelrail-cli nodes mesh` for a handshake per peer. See [mesh requirements](/multi-node#multi-node-mesh-requirements).
- Cloud provisioning against real accounts. See [what was not tested](/node-provisioning#what-was-not-tested).

## If the control plane dies

A container placed on a remote node keeps running, because the agent owns it through its own Docker connection. The agent takes no destructive action on disconnect and retries with exponential backoff until the control plane returns. That node's services are skipped for reconcile passes until then and report `Ready=False` with reason `NodeOffline`. When the mesh runs inside the agent, ingress cannot reach them either, so they stop serving through the control plane until the agent returns (about 35 seconds after an agent restart). [Resilience](/resilience) has the measured numbers, and [Resilience summary](/resilience-summary) has the overview.

## Troubleshooting

### Agent cannot connect to the control plane

The agent enrolled but cannot reach the control plane address and stays "pending".

Likely cause: APP_AGENT_ADVERTISE_HOST on the control plane is set to 127.0.0.1 (the default), which works only for single-node setups. Or, the agent was given the wrong address in APP_CONTROL_PLANE_ADDR.

How to confirm: on the control plane, run `grep APP_AGENT_ADVERTISE_HOST /etc/systemd/system/levelrail.service.d/*`. If it is missing or 127.0.0.1, or if the agent was given a different host, enrollment will fail TLS verification. Check the agent logs with `docker logs levelrail-agent | grep -i tls` or `journalctl -u levelrail-agent | grep -i tls`.

Fix: on the control plane, set APP_AGENT_ADVERTISE_HOST to the control plane's actual hostname or IP (the one the agent can reach, not an internal 127.0.0.1). Edit the systemd unit with `sudo systemctl edit levelrail` and add `Environment=APP_AGENT_ADVERTISE_HOST=<host>`, then restart. Re-enroll the agent with a new token.

### "Never connected" appears after the token is spent

The enrollment succeeded (the token was used) but the node never connected.

Likely cause: the agent crashed or could not reach the control plane. See "Agent cannot connect to the control plane" above.

How to confirm: check the agent logs on the node (docker logs or journalctl) for error messages.

Fix: delete the node from the Nodes page or with `levelrail-cli nodes delete <node-id>`, mint a new join token, and try again with the diagnostics above.

### Workloads on the new node stop after the control plane restarts

Apps or databases placed on a new node are not recovering their desired state after the control plane comes back online.

Likely cause: this is expected during the connection re-establishment window. The agent keeps the workload running, but the control plane needs to reconnect and resume reconciliation.

How to confirm: wait a few seconds and check `levelrail-cli nodes health <node-id>`. Once heartbeat is Recent, the control plane resumes management.

Fix: no action needed. If the node is offline for more than a few minutes, the node status will show Offline, but containers keep running. Once the node reconnects, the control plane resumes managing them.

Still stuck? Check the [Multi-node](/multi-node) troubleshooting section or run `levelrail-cli nodes health <node-id>` and attach its output to an issue on [GitHub](https://github.com/glincker/levelrail/issues).

## Next steps

<CardGroup cols={2}>
<Card title="Multi-node" href="/multi-node">

Enrollment, placement, health, certificates, mesh and the [CLI reference](/cli-reference).

</Card>
<Card title="Node provisioning" href="/node-provisioning">

Provider credentials, cost and known gaps.

</Card>
<Card title="Build node routing" href="/build-node-routing">

Keep builds off nodes that serve traffic.

</Card>
<Card title="Network topology" href="/network-topology">

See nodes, apps and databases on the mesh.

</Card>
</CardGroup>
