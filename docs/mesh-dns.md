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
- Answers only containers on those bridges and the mesh. Any other source gets REFUSED.
- Adds an INPUT accept for udp and tcp 53 from each managed bridge interface, tagged with a comment so it can be found and removed. It uses iptables, or nft when iptables is missing. Turning the setting off removes it.
- Sets the DNS servers of app containers created from then on to the gateway address plus a fallback (see below), but only after that listener is confirmed. Existing containers pick it up when they are recreated.

The bridge list is re-read every `dns.mesh_resolver_interval` (15 seconds by default), so a new network gets a listener and a rule without a restart.

## Public names

Names outside the mesh zone are forwarded to the host's own upstream resolvers, so a container that points at the gateway keeps resolving public names. With `dns.forward_enabled` off, the gateway refuses them and the container relies on its fallback server alone.

- Upstreams come from `dns.forward_upstreams` when set, else from `/run/systemd/resolve/resolv.conf` when that file exists, else `/etc/resolv.conf`. The systemd loopback stub is skipped when the real upstream file exists, and the gateway's own addresses are never used, so the host resolver cannot point back at it.
- Queries go over UDP and retry over TCP when the answer is truncated. An upstream that times out or fails the network is tried last next time. SERVFAIL and REFUSED from an upstream are passed on as answers and do not count against it.
- Forwarded queries carry a small EDNS option naming this node. A query that arrives with this node's own tag is dropped and counted, which breaks a resolver loop.
- Answers are cached in memory, up to `dns.cache_bytes` (4 MiB by default, 0 turns the cache off). TTLs are clamped by `dns.cache_max_ttl`. A name-does-not-exist answer is cached only when it carries an SOA record, for at most `dns.cache_negative_max_ttl`. SERVFAIL is never cached.
- Each container may cause `dns.rate_limit_qps` upstream lookups per second (`dns.rate_limit_burst` at once). Cached answers do not count. Excess queries get REFUSED.
- The query log shows the reasons `forwarded`, `cached` and `upstream_failed` next to the existing ones.

| Setting | Default | Meaning |
| --- | --- | --- |
| `dns.forward_enabled` | true | Forward public names (only active while the mesh resolver is on) |
| `dns.forward_attempt_timeout` | 800ms | Wait for one upstream before trying the next |
| `dns.forward_query_timeout` | 2s | Total time for one lookup, then SERVFAIL |
| `dns.forward_upstreams` | empty | Explicit upstream list, empty means the host resolv.conf |
| `dns.cache_bytes` | 4194304 | Answer cache budget, 0 is off |
| `dns.cache_max_ttl` | 1h | Longest a cached answer lives |
| `dns.cache_negative_max_ttl` | 5m | Longest a cached negative answer lives |
| `dns.container_fallback` | host_upstream | Second DNS server for new containers, or none to fail closed |
| `dns.rate_limit_qps` | 200 | Upstream lookups per second per container, 0 is off |
| `dns.rate_limit_burst` | 400 | Burst size per container |

Each has an environment variable `LEVELRAIL_` plus the upper case key (for example `LEVELRAIL_DNS_CACHE_BYTES`) that only seeds the first value.

## When it falls back

The control plane never points a container at an address that is not listening.

- If port 53 on a gateway address is taken, that listener is skipped, a log line carries the reason `dns_bind_in_use`, and containers there keep Docker's resolver. The mesh server on its own unprivileged port keeps running.
- `dns_bind_denied` means the process may not bind port 53; run it as root or lower the unprivileged port start.
- New containers also get a second DNS server: the first host upstream that a container can reach (`dns.container_fallback`, `host_upstream` by default). When the Levelrail process is stopped or restarted, nothing listens on the gateway, the connection is refused at once, and Docker moves on to that server, so lookups keep working. The honest limit: queries during that gap go to the fallback and are not attributed to an app in the query log. A host whose only resolver is the loopback stub has no usable fallback, so those containers get the gateway alone. Set `none` to keep only the gateway.
- Remote nodes are never given the control plane's gateway address, because no agent runs a resolver yet. Their containers keep Docker's defaults.

## Firewall note

Many hosts end the INPUT chain in a REJECT rule (the default on several cloud images). Without the accept, containers cannot reach a listener on the gateway, and each lookup waits several seconds. If you manage the firewall yourself, add an accept for udp and tcp 53 from the Levelrail bridge interfaces.

## Check it

`levelrail-cli doctor` shows three rows while the setting is on: whether the listeners are bound (with the coded reason when not), whether the INPUT accept is present on every managed bridge, and how many app containers on this node use the resolver.

To see which app asked for what, turn on [DNS query visibility](/dns-visibility).
