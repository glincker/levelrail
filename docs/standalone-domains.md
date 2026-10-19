---
title: "Standalone domains"
description: "Proxy a domain to a port on this server, a network address, an external URL, another app's port or a container, with no app needed, and find the DNS target."
"og:url": "https://docs.levelrail.com/standalone-domains"
---

A standalone domain is a hostname that is not linked to any app. The embedded ingress terminates TLS for it and proxies every request to a target you choose:

| Target | Example | When to use it |
| --- | --- | --- |
| A port on this server | `3000` or `127.0.0.1:3000` | A service you run outside Levelrail on the same machine (a systemd unit, a dev server, an admin tool) |
| An address | `10.0.0.5:8080`, `[fd12::5]:80`, `db.internal:5000` | Something on a private network or the WireGuard mesh |
| A URL | `https://backend.example.org/base` | An external HTTP service; TLS and the `Host` header follow the URL, a path becomes a prefix |
| An app's port | `app:web` or `app:web:9090` | A second hostname for an app, or a port the app does not route by default |
| A container | `container:redis-ui:8081` | A container on this server that Levelrail does not manage |

Standalone domains have the same features as normal app domains: DNS checks, go-live and automatic DNS records, certificate issuance, domain doctor, redirects, maintenance mode, basic auth, WAF and rate limiting, custom error pages, search visibility, and [traffic controls](/domain-traffic-controls) (headers, forwarders, country rules, caching).
## Add one from the dashboard

Open **Domains**, click **Add domain**, and pick **Proxy to a port or URL**. Type the domain, pick the target, and check the live preview (for example `https://tools.example.com -> http://127.0.0.1:3000`). On Linux the port picker lists the ports something is listening on.

**Advanced options** holds the rest: TLS to the target (with certificate verification on or off for self-signed upstreams), the TLS server name, the `Host` header sent upstream (keep the visitor's, use the target's, or a fixed name), websockets (on by default), connect and read timeouts, and a path prefix to strip before proxying.

After you add it, the dialog shows the exact DNS record to create and the live DNS status, then runs go-live: with a connected DNS provider the record is created for you.

In the Domains list a standalone domain shows an **Unlinked** badge with its target in the App column. Its actions menu edits the target or removes the domain; the domain page carries the same tabs as any other domain.

## Add one from the CLI

```
levelrail-cli domains standalone add tools.example.com --target 127.0.0.1:3000 --confirm
levelrail-cli domains standalone add docs.example.com --target https://backend.example.org --read-timeout 60
levelrail-cli domains standalone update tools.example.com --strip-prefix /app --no-websocket
levelrail-cli domains standalone list
levelrail-cli domains standalone remove tools.example.com
```

`add` prints the DNS record to create. Every per-domain command takes `_standalone` as the app name for a standalone domain:

```
levelrail-cli domains basic-auth set _standalone tools.example.com --username ops --password '...'
levelrail-cli domains doctor _standalone tools.example.com
levelrail-cli domains go-live _standalone tools.example.com
```

## Safety rules

A standalone domain lets anyone on the internet reach whatever the target is, so the API is strict about what it accepts:

- Creating, editing or removing one needs the `write:sensitive` ability, the same tier as firewall rules. Every change is recorded in the audit log and shows in the domain's activity timeline.
- Cloud metadata and link-local addresses (`169.254.0.0/16`, `fe80::/10`, `fd00:ec2::254`) are always refused, including through a hostname that resolves to one. Names are resolved again on every ingress pass, so a DNS change cannot slip one in later.
- Loopback, private, mesh and container targets are allowed on purpose, but need an explicit confirmation (`confirm: true` in the API, `--confirm` in the CLI, a checkbox in the dashboard) that names the address this server will connect to. Editing other settings later does not ask again; a new target does.
- This platform's own dashboard and agent ports need the same confirmation. The ingress admin API port can never be a target.
- A target that resolves to this server's own public address on 80 or 443, or to a local ingress port, is refused as a loop.
- The domain must be free: an app, environment, preview, static site, the dashboard's primary domain, the built-in registry, the [status page](/status-page) or an AI model already using it is named in the error.

Standalone domains are instance wide. Members of non-default teams cannot list or change them; the routes answer 403 for them.

## Server address and DNS records

The **Server address** card on the Domains page shows this server's public IPv4 and IPv6 with copy buttons. They come from `APP_PUBLIC_HOST` or detection (`APP_PUBLIC_IP_DETECT`), and you can override them by hand when the server sits behind a [load balancer](/load-balancing) or detection is wrong. The override also drives every DNS check and go-live.

The card warns when:

- the public IPv4 is not on any network interface here, which means NAT or a cloud address mapping: ports 80 and 443 must be forwarded to this server;
- a proxy in front terminates TLS, in which case DNS should point at that proxy.

From the CLI:

```
levelrail-cli system address
levelrail-cli system address --domain tools.example.com
levelrail-cli system address --ipv4 203.0.113.4 --ipv6 2001:db8::4
levelrail-cli system address --clear
```

With `--domain` it prints the exact records to create: `A` and `AAAA` for the domain, or a `CNAME` to the apps base domain as an alternative when one is set.

## Behind another proxy

When a proxy such as Traefik in front owns ports 80 and 443 and [managed proxy routes](/domains-and-ingress) are on, standalone domains get a route file in that proxy's directory like app domains do, pointing at the ingress listener.

## Troubleshooting

### The target is unreachable even though the domain resolves

The domain points to the server, but requests are not reaching the target.

Likely cause: the target address is wrong, the service is not listening on that port, or a firewall is blocking it.

How to confirm: verify the target with `levelrail-cli domains standalone list` or the Domains page. Also try connecting to the target directly from the server: `curl -v http://127.0.0.1:3000` (or whatever the target is).

Fix: if the service is not running, start it. If the port is wrong, update the standalone domain's target with `levelrail-cli domains standalone update <domain> --target <correct-target>`. If a firewall is blocking it, open the port (for internal addresses on the mesh, ensure the mesh is on and working).

### Adding a private network target is rejected with an error

The API rejected the target because it is a cloud metadata or link-local address.

Likely cause: cloud metadata addresses (like AWS `169.254.169.254`) and link-local addresses cannot be proxied for security reasons.

How to confirm: check the error message when adding the domain. It will name which address category was blocked.

Fix: use a different target. For cloud metadata, access it directly on the service instead. For link-local addresses, use a mesh address or a hostname that resolves to a non-link-local IP.

### A loopback or private address target requires confirmation but I already confirmed once

Confirmation was needed when the target was first created, but editing other settings later asks again.

Likely cause: you changed the target address. Every new target to a sensitive address needs confirmation.

How to confirm: try updating the domain with a new target, and an error appears asking for confirmation.

Fix: pass `--confirm` or include `"confirm": true` in the API request to acknowledge the new target address.

### The domain tries to proxy to itself and returns a loop error

The target resolves to this server's public address on ports 80 or 443, creating an infinite loop.

Likely cause: the target address is the same as this server's public IP, which would cause traffic to loop back.

How to confirm: check what the target resolves to with `nslookup` or `dig`, and compare it with the server address shown on the Domains page.

Fix: change the target to an address that is not this server's public IP. If you want to proxy to a local service, use `127.0.0.1:<port>` instead of the public address. If you need the target to be external, use a different hostname.

Still stuck? Run `levelrail-cli domains standalone list --json` and attach its output to an issue on [GitHub](https://github.com/glincker/levelrail/issues).

## API

| Method and path | Ability | What it does |
| --- | --- | --- |
| `GET /api/v1/domains/standalone` | read | List, or `?domain=` for one |
| `POST /api/v1/domains/standalone` | write:sensitive | Create |
| `PUT /api/v1/domains/standalone/{domain}` | write:sensitive | Update; omitted fields keep their value |
| `DELETE /api/v1/domains/standalone/{domain}` | write:sensitive | Remove, along with its per-domain settings |
| `POST /api/v1/domains/standalone/validate` | write:sensitive | Dry run: target URL, resolved addresses, what needs confirmation |
| `GET /api/v1/domains/standalone/candidates` | write:sensitive | Listening host ports, running containers and apps |
| `GET /api/v1/system/server-address` | read | Public addresses, source, NAT note, `?domain=` adds DNS records |
| `PUT /api/v1/system/server-address` | root | Set or clear the override |

`GET /api/v1/domains` includes standalone domains with `kind: "standalone"` and `target`. A create body looks like:

```json
{
  "domain": "tools.example.com",
  "target": "127.0.0.1:3000",
  "upstream_tls": false,
  "tls_skip_verify": false,
  "host_header": "preserve",
  "websocket": true,
  "dial_timeout_seconds": 0,
  "read_timeout_seconds": 60,
  "strip_prefix": "/app",
  "confirm": true
}
```

A target that needs confirmation answers `409` with `needs_confirmation`, a list of plain sentences naming what this server would connect to.
