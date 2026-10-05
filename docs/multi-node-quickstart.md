---
description: The short path to a second server, what has been verified on real infrastructure, and what to check before relying on the WireGuard mesh across hosts.
---

# Multi-node quickstart

This page is the short path to a second node. Each step links to the page that holds the detail, so nothing is repeated here. It also says plainly what has and has not been verified, per [Feature status](/feature-status).

<Steps>
<Step title="Get a second machine">

Pick one of two paths:

- **Bring your own server.** Any Linux box with Docker, systemd and outbound network access works. Use [Enrolling over SSH](/multi-node#enrolling-over-ssh-instead-of-by-hand) to have the control plane set it up, or [enroll it by hand](/multi-node#enrolling-a-second-node).
- **Create one at a cloud provider.** Hetzner, DigitalOcean, AWS, Azure and GCP are supported. See the [multi-cloud provisioning quickstart](/multi-cloud-provisioning) for the command sequence and [Node provisioning](/node-provisioning) for credentials and cost.

</Step>
<Step title="Set the advertise host before enrolling">

Set `APP_AGENT_ADVERTISE_HOST` on the control plane to the host or IP the remote agent will dial. It defaults to `127.0.0.1`. With a mismatch, enrollment succeeds but the node's session then fails TLS hostname verification on every attempt, and the node stays `pending`. Details are in [multi-node](/multi-node#enrolling-a-second-node).

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

- **A real WAN or second physical host.** Both daemons ran on one machine, so cross-datacenter latency and NAT traversal are untested.
- **The WireGuard mesh between two real hosts.** The mesh code is tested against fakes and a single host with a real TUN device. If you turn it on across hosts, open inbound UDP 51820 on the control plane and check `levelrail-cli nodes mesh` for a handshake per peer. See [mesh requirements](/multi-node#multi-node-mesh-requirements).
- **Cloud provisioning against real accounts.** See [what was not tested](/node-provisioning#what-was-not-tested).

## If the control plane dies

A container placed on a remote node keeps running and serving, because the agent owns it through its own Docker connection. The agent takes no destructive action on disconnect and retries with exponential backoff until the control plane returns; that node's services are skipped for reconcile passes until then. [Resilience](/resilience) has the measured numbers, and the [short version](/resilience-summary) has the summary.

## Next steps

<CardGroup :cols="2">
<Card title="Multi-node" href="/multi-node">

Enrollment, placement, health, certificates, mesh and the CLI reference.

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
