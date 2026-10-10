---
description: Find which published container ports are reachable from the internet, why ufw does not cover them, and how to restrict one with a reversible DOCKER-USER allow-list.
---

# Exposure audit

Docker publishes a container port by adding NAT rules that send traffic straight to the container. That traffic never reaches the INPUT chain, so `ufw` and a "firewall is active" check say nothing about it. A search engine or database container published on `0.0.0.0` is reachable by anyone, even on a host with `ufw` on and a deny default.

The exposure audit lists, for each node, every running container's published ports and how reachable each one is. It is read-only. Nothing is changed until you choose to restrict a port.

<InlineToc default-open />

## What each state means

| State | Meaning |
| --- | --- |
| This server only | Bound to `127.0.0.1` or `::1`. Nothing outside can connect. |
| Private network | Bound to a private, link-local or CGNAT (mesh) address. |
| Restricted | A simple DROP or REJECT rule in the DOCKER-USER chain matches the port. The allowed sources are listed. |
| Exposed | Published on every interface (or a public address) and no matching rule was found. |
| Unknown | Published publicly, but the chain could not be read or a rule uses matches the audit cannot interpret. |

The audit never calls a port closed on a guess. A rule with an interface match, an ipset or a custom chain jump is reported as Unknown, not Restricted.

## Severity

Severity follows what is exposed, not just that something is.

- **High**: a database, key-value store, search engine, admin console or the Docker API, published publicly without an opt-in.
- **Medium**: the same kinds when you opted in (a database with public access enabled), or an unrecognised service from an unmanaged container.
- **Low or info**: a web port. Apps behind the ingress are expected to be reachable, ideally only through it.

The owner is shown too: a managed database, a managed app, or an unmanaged container. A managed database bound publicly without public access enabled is a drift worth fixing.

## Where it shows up

- **Settings, Firewall**: the Published ports table, with an expandable plain explanation per row.
- **Doctor and setup**: an `exposure` check group in the Network and ingress category, one finding per high or medium port.
- **CLI**: `levelrail-cli firewall exposure`.
- **MCP**: `get_firewall_exposure`, read-only. No MCP tool can apply or remove a rule.

```bash
levelrail-cli firewall exposure
levelrail-cli firewall exposure --node worker-1 --json
levelrail-cli firewall exposure --probe
```

## Check from outside

`--probe` (or "Check from outside" in the dashboard) dials each internet-facing TCP port on the node's public address. For the control plane's own host that dial comes from the host itself, and many routers block a server from reaching its own public address (hairpin NAT). So a failed dial is reported as "could not confirm", never as closed, and a successful one is reported as "answered from this side", which does not prove a cloud firewall in front of the host agrees.

## Restrict a port

Restricting a port installs an allow-list: the sources you name can connect, everyone else is dropped.

```bash
levelrail-cli firewall restrict --port 8108 --allow 203.0.113.7,10.0.0.0/8 --dry-run
levelrail-cli firewall restrict --port 8108 --allow 203.0.113.7,10.0.0.0/8 --apply
levelrail-cli firewall unrestrict --port 8108
```

`--local-containers` additionally allows Docker's default address pool (`172.16.0.0/12`). If you use a custom pool, list its CIDR explicitly.

### What gets written

For each restriction the control plane inserts, at the top of `DOCKER-USER`, one RETURN rule per allowed source followed by one DROP rule. Each rule is tagged with a comment `<brand prefix>exposure:<proto>/<port>` (for example `levelrail:exposure:tcp/8108`). They match the original destination port with `conntrack --ctorigdstport`, because DOCKER-USER sees traffic after NAT, when the destination port is already the container's port. Only the original direction is matched, so replies are not dropped.

### Safety

- **Dry run first.** The preview shows the exact `iptables` commands and a sentence stating what traffic will be dropped. Applying requires explicit confirmation.
- **Lockout guard.** A restriction is refused for the management API, the agent listener, ingress ports 80 and 443, and SSH. Known nodes that are not in your allow-list are listed as warnings.
- **Only its own rules.** Removal and re-assertion touch only rules carrying the tag. Rules you wrote are never changed or deleted.
- **Reversible.** One click or `unrestrict` removes the rules and forgets the restriction.
- **Audited.** Apply and remove go through the audit log with the port in the request path, and need the root ability. The audit and preview are plain reads.
- **Off by default.** Nothing is applied unless you ask.

### Persistence

Rules in DOCKER-USER live in the kernel only. The restriction is stored in the control plane database and a reconciler re-applies it on every pass, so it comes back after a reboot or a Docker restart while the control plane runs on that host. If you stop the control plane, rules stay until the next reboot.

### Limits

- Rules can be applied only on the control plane's own host. For a remote node, copy the dry-run commands and run them there. An agent RPC for this is planned.
- Only IPv4 sources and the iptables backend are supported. Docker's experimental nftables mode has no DOCKER-USER chain and is reported as unreadable.
- Reading the chain needs root, the same as turning on `ufw`. Without it, wildcard-bound ports are reported as Unknown.
- On macOS or Docker Desktop the host firewall is not visible, so publicly bound ports are Unknown.
