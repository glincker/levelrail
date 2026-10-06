---
description: Manage allow and deny rules for the control plane host's ufw firewall from the dashboard, CLI or API, how the reconciler applies them, and the lockout guard that refuses a rule on a port the platform needs.
---

# Host firewall

Levelrail can manage a set of allow and deny rules for the **control plane host's own `ufw` firewall**. You declare rules as records (a port, a protocol, an optional source, an action), and a reconciler converges `ufw` to match them. It is the same declarative model as the rest of the platform: edit the record, and the host follows.

This is opt-in and narrow by design. If you never add a rule, nothing is changed on the host.

<InlineToc default-open />

## What it does and does not touch

- **Only tagged rules.** Every rule the platform creates carries a `ufw` comment prefix derived from the brand short name (`levelrail:` by default, see [White-labeling](white-labeling.md)). Sync only adds and removes rules carrying that prefix. Rules you created yourself with `ufw` are never changed or deleted.
- **Enables the firewall only when you ask.** Nothing is switched on automatically. Use **Settings, Firewall, Turn on firewall** (or `levelrail-cli firewall enable`) and the platform allows SSH first, then the control plane ports and HTTP/3, and only then runs `ufw --force enable`. It never changes the default policy by itself, and if any allow step fails, the firewall stays off. If `ufw` is installed but inactive, rules are stored but not applied until you turn it on.
- **Only `ufw`, only the local host.** If `ufw` is not installed (you use firewalld, nftables, iptables or a cloud security group), nothing is applied and the reconciler reports that as informational, not a failure. Rules are stored for the control plane host. Per-node rules for remote nodes are not supported yet, and a rule pinned to a remote node would be skipped and logged.
- **Not the installer's firewall step.** `install.sh` can allow SSH, 80 and 443 once with `LEVELRAIL_CONFIGURE_UFW=1` (see [Installing](installing.md)). That is a one-time script action. This page covers the ongoing, managed rules.

## The lockout guard

A deny rule, or an allow rule restricted to a source CIDR, on a port the control plane itself needs is **refused** rather than applied. The protected ports are the ones this instance is actually configured to use:

- the management API address (`APP_HTTP_ADDR`),
- the agent gRPC address (`APP_AGENT_ADDR`),
- the ingress HTTP address (`APP_INGRESS_HTTP_ADDR`),
- the ingress HTTPS address (`APP_INGRESS_HTTPS_ADDR`).

With defaults these are 8080, 9443, 80 and 443. An unrestricted allow on a protected port is accepted. The check runs when you create the rule (the API returns a validation error) and again in the reconciler as a second line of defense. A control plane that has locked itself out cannot reopen itself, which is why this is refused unconditionally.

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

## Add and remove rules

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

Open **Settings**, then **Firewall**. The page lists each rule with its action, port, protocol, source and label, and shows the host's firewall preflight result from the system doctor. Use the add action to create a rule. Creating a deny rule, or an allow rule restricted to a source, shows a confirmation that explains it will close or narrow a port.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli firewall list
levelrail-cli firewall allow --port 5432 --source-cidr 10.0.0.0/8 --label "postgres from the office VPN"
levelrail-cli firewall deny --port 3306 --protocol tcp --label "block mysql"
levelrail-cli firewall delete <id>
```

`allow` and `deny` take `--port` (required, 1 to 65535), `--protocol` (`tcp` or `udp`, default `tcp`), `--source-cidr` (default: any source) and `--label`. `delete` takes the rule ID printed by `list` and `allow` or `deny`. All subcommands accept the standard `--token`, `--api-url`, `--profile`, `--json`, `--output` and `--query` flags.

</Tab>
<Tab value="API">

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

## Related

<CardGroup :cols="2">
<Card title="Security overview" href="/security">

Fresh-box hardening checklist and the doctor's firewall check.

</Card>
<Card title="Domains and ingress" href="/domains-and-ingress">

Ports 80 and 443 for ingress and ACME.

</Card>
</CardGroup>
