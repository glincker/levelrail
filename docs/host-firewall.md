---
title: "Host firewall"
description: Manage allow and deny rules for the control plane host's ufw firewall from the dashboard, CLI or API, with a lockout guard against blocking essential ports.
"og:url": "https://docs.levelrail.com/host-firewall"
---

Levelrail can manage allow and deny rules for the control plane host's own `ufw` firewall (`ufw` is the firewall front end that ships with Ubuntu). You declare rules as records (a port, a protocol, an optional source, an action), and a reconciler changes `ufw` to match them. Edit the record and the host follows, like everything else on the platform.

This is opt-in and narrow. If you never add a rule, nothing on the host changes.

## What it does and does not touch

- Only tagged rules: every rule the platform creates carries a `ufw` comment prefix derived from the brand short name (`levelrail:` by default, see [White-labeling](/white-labeling)). Sync only adds and removes rules carrying that prefix. Rules you created yourself with `ufw` are never changed or deleted.
- Enables the firewall only when you ask: nothing is switched on automatically. Use **Settings, Firewall, Turn on firewall** (or `levelrail-cli firewall enable`) and the platform allows SSH first, then the control plane ports and HTTP/3, and only then runs `ufw --force enable`. It never changes the default policy by itself, and if any allow step fails, the firewall stays off. If `ufw` is installed but inactive, rules are stored but not applied until you turn it on.
- Only `ufw`, only the local host: if `ufw` is not installed (you use firewalld, nftables, iptables or a cloud security group), nothing is applied and the reconciler reports that as informational, not a failure. Rules are stored for the control plane host. Per-node rules for remote nodes are not supported yet, and a rule pinned to a remote node would be skipped and logged.
- Not the installer's firewall step: `install.sh` can allow SSH, 80 and 443 once with `LEVELRAIL_CONFIGURE_UFW=1` (see [Installing](/installing)). That is a one-time script action. This page covers the ongoing, managed rules.

## The lockout guard

Levelrail refuses a deny rule, or an allow rule restricted to a source CIDR, on a port the control plane itself needs. The protected ports are the ones this instance is actually configured to use:

- the management API address (`APP_HTTP_ADDR`),
- the agent gRPC address (`APP_AGENT_ADDR`),
- the ingress HTTP address (`APP_INGRESS_HTTP_ADDR`),
- the ingress HTTPS address (`APP_INGRESS_HTTPS_ADDR`).

With defaults these are 8080, 9443, 80 and 443. An unrestricted allow on a protected port is accepted. The check runs when you create the rule (the API returns a validation error) and again in the reconciler as a second line of defense. A control plane that has locked itself out cannot reopen itself, so this is always refused.

## Turn the firewall on or off

The switch needs the root ability and is audit logged. Turning it on first shows the exact commands, then runs them in order: SSH, the management, agent and ingress ports, and 443/udp for HTTP/3, followed by `ufw --force enable`. The onboarding Server check offers the same button, so a new server never needs a command pasted into a terminal.

SSH is assumed to be on port 22. If yours is not, set `APP_FIREWALL_SSH_PORTS` (comma separated) on the control plane before turning the firewall on.

```bash
levelrail-cli firewall status
levelrail-cli firewall enable --dry-run   # print the commands, change nothing
levelrail-cli firewall enable
levelrail-cli firewall disable
```

The API is `GET /api/v1/firewall/host`, `POST /api/v1/firewall/host/enable` and `POST /api/v1/firewall/host/disable`, where both POSTs accept `{"dry_run": true}`.

### Ports apps publish directly

`GET /api/v1/firewall/host` also returns `published_ports`: one row per host port an app binds directly, TCP or UDP, with `port`, `protocol`, `app`, `service`, `bind_address` and `allowed`. `allowed` is true when the host firewall lets that port and protocol in today (ufw absent, inactive, defaulting to allow, or with a matching allow rule). A cloud security group is not visible here, so check it separately for UDP ports such as 53 or 51820. Ports bound beyond loopback are added to the turn-on commands with their own protocol, so enabling the firewall never cuts off a running UDP service.

## Add and remove rules

<Tabs>
<Tab title="Dashboard">

Open **Settings**, then **Firewall**. The page lists each rule with its action, port, protocol, source and label, and shows the host's firewall preflight result from the system doctor. Use the add action to create a rule. Creating a deny rule, or an allow rule restricted to a source, shows a confirmation that explains it will close or narrow a port.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli firewall list
levelrail-cli firewall allow --port 5432 --source-cidr 10.0.0.0/8 --label "postgres from the office VPN"
levelrail-cli firewall deny --port 3306 --protocol tcp --label "block mysql"
levelrail-cli firewall delete <id>
```

`allow` and `deny` take `--port` (required, 1 to 65535), `--protocol` (`tcp` or `udp`, default `tcp`), `--source-cidr` (default: any source) and `--label`. `delete` takes the rule ID printed by `list` and `allow` or `deny`. All subcommands accept the standard `--token`, `--api-url`, `--profile`, `--json`, `--output` and `--query` flags.

</Tab>
<Tab title="API">

| Method | Path | Notes |
| --- | --- | --- |
| `GET` | `/api/v1/firewall-rules` | List rules. Read ability. |
| `POST` | `/api/v1/firewall-rules` | Create a rule. Sensitive write ability. Body fields: `port` (required), `protocol`, `source_cidr`, `action`, `label`. |
| `DELETE` | `/api/v1/firewall-rules/{id}` | Delete a rule. Sensitive write ability. Returns 204. |

On create, `protocol` defaults to `tcp` and `action` defaults to `allow`. A rule that fails validation, including the lockout guard, returns 400 with the reason.

</Tab>
</Tabs>

A source CIDR must parse as a valid CIDR. A source of `0.0.0.0/0` is treated the same as no source (any source).

## How rules are applied

Creating or deleting a rule nudges the reconciler, which reads every stored rule, re-validates each against the protected ports, and syncs `ufw`: it adds tagged rules that are missing and removes tagged rules that no longer have a record. Each applied rule's comment is the prefix plus `rule:<id>`, so you can match a `ufw status` line back to its record.

The reconciler reports one of these outcomes as the firewall controller's status:

| Outcome | Meaning |
| --- | --- |
| Synced N rules | Every stored rule is in place. The message also reports how many were applied and removed on that pass. |
| `UFWNotInstalled` | `ufw` is not on the host. Nothing is managed. |
| `UFWInactive` | `ufw` is installed but not enabled. Nothing is managed until you enable it yourself. |
| `RuleSyncFailed` | One or more `ufw` commands failed. The other rules are still processed, and the message lists the errors. |

## Published container ports

To publish an app on a host port and open it here in one step, see [App ports and exposure](/app-exposure).

`ufw` does not cover ports Docker publishes: they skip the INPUT chain. The [Exposure audit](/exposure-audit) lists which of your containers are reachable from outside and can restrict them with DOCKER-USER rules.

## Troubleshooting

### Ports 80 or 443 report as "blocked" in the preflight check

The installer could not reach the ports from outside the network during the preflight readiness check.

Likely cause: the cloud provider's firewall, the host firewall, or a NAT/hairpin configuration is blocking the ports.

How to confirm: run `levelrail-cli readiness [--domain <domain>]` or check Settings > Updates > Server readiness in the dashboard to see which ports are unreachable. Also try `curl -v http://localhost:80` and `curl -v https://localhost:443` on the server itself to confirm the control plane is listening.

Fix: open ports 80 and 443 (both TCP; 443 also UDP for HTTP/3) in your cloud provider's firewall, and in the host firewall with `sudo ufw allow 80,443/tcp` or `sudo ufw allow 443/udp`. If the server is behind a NAT or hairpin NAT is not available, test from outside the network instead. Then re-run the readiness check.

### A container port is not reachable from outside

A container published a port but external traffic cannot reach it.

Likely cause: ufw is blocking the port (it does not affect Docker's DOCKER-USER chain by default), or the container was not actually published.

How to confirm: run `docker ps` and check if the container has a port mapping (e.g., `0.0.0.0:8000->8000/tcp`). Also try connecting from the server itself (localhost) vs from outside to narrow down whether it is a local or network firewall issue.

Fix: if ufw is blocking it, add the port with `sudo ufw allow <port>/tcp` (or `udp`). If Docker is not publishing the port, check the container's run config or deployment spec. Verify with `ss -tlnp | grep <port>` to see what is listening and who owns it.

Still stuck? Run `levelrail-cli readiness` and attach its output to an issue on [GitHub](https://github.com/glincker/levelrail/issues).

## Related

<CardGroup cols={2}>
<Card title="Security overview" href="/security">

Fresh-box hardening checklist and the doctor's firewall check.

</Card>
<Card title="Domains and ingress" href="/domains-and-ingress">

Ports 80 and 443 for ingress and ACME.

</Card>
</CardGroup>
