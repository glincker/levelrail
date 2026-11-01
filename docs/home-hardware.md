---
title: "Run Levelrail on home hardware"
sidebarTitle: "Home hardware"
description: "Install notes for a Raspberry Pi, an old laptop or a mini PC, and what to do when your home connection has no public address."
"og:url": "https://docs.levelrail.com/home-hardware"
---

Levelrail runs on ordinary Linux machines, so a spare computer at home is a fair place to try it. This page covers what the code supports, the install, and the one problem that most home connections have: no public address.

<Warning>
Nothing on this page has been run on a Raspberry Pi, an old laptop or a mini PC by the Levelrail team. Statements about the code are checked against the source. Anything about real hardware is marked "not yet verified on real hardware".
</Warning>

## What is supported

- **Architectures: `amd64` and `arm64`.** The server readiness check fails any other architecture, and so does the node preflight (`APP_NODE_PREFLIGHT_ARCHITECTURES` defaults to `amd64,arm64`). A 64-bit Raspberry Pi OS or Ubuntu on a Pi 4 or Pi 5 reports `arm64`. A 32-bit OS reports `armv7l` and is not supported.
- **Operating system:** Linux with systemd and root access, as in [Getting started](/getting-started).
- **Memory:** 1 vCPU and 1 GB of RAM boots the platform. 2 GB is more comfortable once you build images on the same machine. How much a given app needs on a Pi is not yet verified on real hardware.
- **Disk:** the node preflight wants 5 GB free by default (`APP_NODE_PREFLIGHT_MIN_DISK_FREE_MB`). Prefer an SSD to an SD card for database and image storage. Whether an SD card holds up is not yet verified on real hardware.

Check the architecture before you start:

```bash
uname -m   # x86_64 or aarch64 is fine, armv7l is not
```

## Install

On the machine itself, as in [Installing](/installing):

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

The install on a Pi 4 or Pi 5, an old laptop and a mini PC is the same command. It is not yet verified on real hardware for any of the three.

For a laptop, turn off sleep on lid close and on idle, or the server stops when you walk away. Give the machine a fixed address on your router so the address you use does not change.

## The public address problem

Let's Encrypt and the default `sslip.io` address need the internet to reach ports 80 and 443 on your machine. Many home connections cannot allow that:

- The provider uses carrier-grade NAT (CGNAT), so your router has no public IPv4 address of its own.
- The provider blocks inbound ports 80 or 443.
- Your address changes and you have no hostname for it.

Port forwarding does not help with CGNAT. You have three options.

### Option 1: public VPS control plane, home node

Run the control plane on a small public VPS and enroll the home machine as a node. The node dials out to the control plane, and the control plane never opens a connection to the node, so the home connection needs no inbound port for enrollment. See [Multi-node](/multi-node) for how enrollment works and [Add a node anywhere](/add-node-anywhere) for the guided flow, which treats a home server as a normal node.

Cross-node networking uses the [WireGuard mesh](/multi-node#wireguard-mesh-and-internal-dns). Public traffic lands on the VPS, and the VPS reaches apps on the home node over the mesh. Whether the mesh data path works from behind CGNAT is not yet verified on real hardware. Set `APP_AGENT_ADVERTISE_HOST` to the VPS address, as [Multi-node](/multi-node) describes.

### Option 2: wildcard certificate

A wildcard host can get a trusted certificate with DNS-01 and a connected DNS provider (Cloudflare, Route53 or Google Cloud DNS), without any inbound port. Traffic still has to reach the machine, so this suits a home network you reach over a VPN or a LAN. See [Domains and ingress](/domains-and-ingress).

### Option 3: tunnel or uploaded certificate

An experimental Cloudflare Tunnel controller exists behind a flag and has not been run against a real account, so do not rely on it yet. An uploaded certificate works for any host you can reach, with no ACME traffic. See [Domains and ingress](/domains-and-ingress).
