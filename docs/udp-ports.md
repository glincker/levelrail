---
title: "UDP published ports"
description: Publish a UDP port from a compose service directly on the host, with the accepted syntax, safety rules, errors and what is not supported yet.
"og:url": "https://docs.levelrail.com/udp-ports"
---

A compose service can publish a UDP port on the host with `HOST:CONTAINER/udp`. This covers services such as a DNS server or a STUN endpoint. It is the first slice of [ADR 049](https://github.com/glincker/levelrail/blob/release/v0.8.9/adr/049-udp-ports-capabilities-sysctls.md): UDP publishing only.

## What it does

- Publishes the UDP port straight on the host. UDP does not go through the HTTP reverse proxy, which cannot carry it.
- Accepts the short form (`"53:53/udp"`) and the long form (`target`, `published`, `protocol: udp`).
- Treats TCP and UDP on the same number as two separate ports. `53:53/tcp` and `53:53/udp` can both be declared.
- Shows each published port as `53/udp` (or `53/tcp`). The API returns it under `published_ports` on the app and on each compose deploy `services[]` entry:

```json
"published_ports": [{"container_port": 53, "host_port": 53, "protocol": "udp"}]
```

App spec files take the same `published_ports` shape.

## What it does not do

- It works on the control plane's own node only. A service placed on a remote node is refused with `published_port_remote_node`.
- It gives no reachability probe. A UDP port only reports whether it is bound.
- It does not add capabilities, sysctls or host networking (see [Not yet](#not-yet)).

## Safety

- The port binds to the service's bind address, which is private by default, like a TCP host port. Making it public follows the same exposure rules as a TCP host port, see [App ports and exposure](/app-exposure).
- The host or cloud firewall must still allow the port from the clients you want. Publishing does not open it for you.
- A public UDP service is easy to abuse. An open DNS resolver can be used for amplification, so keep it private unless you mean to serve the internet.
- Exposure findings already list UDP ports from Docker.
- The interaction with the Docker `DOCKER-USER` firewall chain has not been tested on a real server yet. Do not rely on a `DOCKER-USER` rule to restrict a UDP port until it is.
- Known gaps: the firewall edge view has no UDP rows yet, and a UDP port is not checked against processes already listening on the host.

## Errors

| Code | Message | What to do |
| --- | --- | --- |
| `published_port_invalid` (400) | `published port 51820/udp: a host port between 1 and 65535 is required, for example "51820:51820/udp"` | Give an explicit host port. A bare `51820/udp` has none. |
| `published_port_invalid` (400) | `published port 53/udp is declared twice` | Remove the duplicate entry. |
| `published_port_conflict` (409) | `host port 53/udp for service "<svc>" is already published by "<other>"` | Pick another host port, or stop the other service. |
| `published_port_remote_node` (400) | `host port 53/udp can only be published on the control plane's own node, service "<svc>" is placed on node "<id>"` | Place the service on the control plane node. |
| none (400, parse) | `protocol "sctp" is not supported, use tcp or udp` | Use `tcp` or `udp`. |

A running deploy that hits the remote node case reports the reconcile reason `PublishedPortsRemoteNode`.

## Example: UDP echo test

```yaml
services:
  echo:
    image: python:3.12-alpine3.20
    command:
      - python3
      - -c
      - |
        import socket
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.bind(("0.0.0.0", 9000))
        while True:
            d, a = s.recvfrom(65535)
            s.sendto(d, a)
    ports:
      - "9000:9000/udp"
```

```bash
levelrail-cli compose deploy echo -f compose.yaml
echo hello | nc -u -w1 <server-ip> 9000
```

The port is private by default, so run the test from the server itself or set a public bind address and open UDP 9000 in the firewall. The echo server is a short Python loop because busybox `nc -e` serves only one client per container start, so a second test would hang.

## Example: DNS-style service

```yaml
services:
  dns:
    image: alpine:3.20
    command:
      - sh
      - -c
      - apk add --no-cache dnsmasq && exec dnsmasq -k --no-resolv --port=53 --interface=eth0 --address=/app.test/10.0.0.5
    ports:
      - target: 53
        published: 5354
        protocol: udp
      - "5354:53/tcp"
```

This publishes container port 53 on host port 5354 for UDP (long form) and for TCP (short form, declared as the same number to show they are separate). Test it with `dig @<server-ip> -p 5354 app.test`, which answers `10.0.0.5`. Host port 5354 avoids a clash with a resolver already on port 53 and with mDNS on 5353. `--interface=eth0` makes dnsmasq answer queries that arrive through the port forward; without it dnsmasq drops them as non-local. The container installs dnsmasq at start, so it needs outbound network access.

## Not yet

These are not built, so compose files that use them are still rejected:

- `cap_add` (for example `NET_ADMIN`)
- `sysctls`
- `network_mode: host`

That means a WireGuard server cannot run as an app yet. See [ADR 049](https://github.com/glincker/levelrail/blob/release/v0.8.9/adr/049-udp-ports-capabilities-sysctls.md) for the plan.
