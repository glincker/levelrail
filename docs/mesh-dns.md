---
title: "Mesh DNS on the bridge gateways (beta)"
description: "Let app containers on this node resolve through the mesh DNS server, with the firewall rule and checks it needs. Off by default."
"og:url": "https://docs.levelrail.com/mesh-dns"
---

**Beta.** This feature is off by default and may change. It needs mesh networking to be on.

Docker only forwards a container's lookups to port 53 of the DNS server address it is given. Without an explicit address, a container asks the host's own resolver, and the mesh DNS server never sees the query. This setting closes that gap on the node that runs the control plane.

## Turn it on

Set `dns.mesh_resolver_enabled` to true (Settings, `levelrail-cli settings set`, or the API). It applies live. The environment variable `LEVELRAIL_DNS_MESH_RESOLVER_ENABLED` only seeds the default.

When it is on, the node:

- Binds the mesh DNS server to each Levelrail-managed bridge gateway address on port 53 (udp and tcp). Never `0.0.0.0`, and never the host's own `127.0.0.53` or `127.0.0.54` stub.
- Answers only containers on those bridges and the mesh. Anything else gets REFUSED.
- Adds an INPUT accept for udp and tcp 53 from each managed bridge interface, tagged with a comment so it can be found and removed. It uses iptables, or nft when iptables is missing. Turning the setting off removes it.
- Sets the DNS server of app containers created from then on to the gateway address, but only after that listener is confirmed. Existing containers pick it up when they are recreated.

The bridge list is re-read every `dns.mesh_resolver_interval` (15 seconds by default), so a new network gets a listener and a rule without a restart.

## When it falls back

The control plane never points a container at an address that is not listening.

- If port 53 on a gateway address is taken, that listener is skipped, a log line carries the reason `dns_bind_in_use`, and containers there keep Docker's resolver. The mesh server on its own unprivileged port keeps running.
- `dns_bind_denied` means the process may not bind port 53; run it as root or lower the unprivileged port start.
- Remote nodes are never given the control plane's gateway address, because no agent runs a resolver yet. Their containers keep Docker's defaults.

## Firewall note

Many hosts end the INPUT chain in a REJECT rule (the default on several cloud images). Without the accept, containers cannot reach a listener on the gateway, and each lookup waits several seconds. If you manage the firewall yourself, add an accept for udp and tcp 53 from the Levelrail bridge interfaces.

## Check it

`levelrail-cli doctor` shows three rows while the setting is on: whether the listeners are bound (with the coded reason when not), whether the INPUT accept is present on every managed bridge, and how many app containers on this node use the resolver.

To see which app asked for what, turn on [DNS query visibility](/dns-visibility).
