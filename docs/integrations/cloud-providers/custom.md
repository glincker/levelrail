---
title: "Use Custom data center with Levelrail: node provisioning"
sidebarTitle: "Custom data center"
description: "Record a data center that has no cloud API and enroll its machines with the same one-time join token flow."
---

Record a data center that has no cloud API and enroll its machines with the same one-time join token flow.

This page covers how to enroll bare metal or any VM as a node.

## What you get

- A named data center that groups machines you manage yourself.
- The same one-time join token and install command as manual enrollment.
- No cloud API is called, so nothing needs a provider credential.

## Set up

1. Prepare the credential. Use the manual method: a name, a location label and notes. There is no secret.
2. In the dashboard, open **Settings > Cloud node providers** (`/settings/node-providers?connect=custom`), then Nodes > Add node.
3. Connect runs a read-only region listing first and stores nothing when it fails, so a saved connection already works.

From the CLI:

```bash
levelrail-cli nodes providers connect --provider custom --label "Rack 4" --location Frankfurt
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| No cloud permissions | No cloud API is called. The machine only needs outbound TCP to the control plane agent port and outbound HTTPS. |

## Endpoint and format

No endpoint. The agent dials out to the control plane.

## Security notes

- A saved credential is envelope encrypted and never shown again, in the API or the audit log.
- Piping a credential on stdin avoids leaving it in shell history; a flag value is visible in the process list.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Levelrail does not create or delete the machine for you.
- Enrollment uses the manual join token flow described in the multi-node guide.

## Troubleshooting

**Connect asks for a label.** A custom data center requires --label. Location and notes are optional.

**The agent never appears as a node.** The machine needs outbound TCP to the control plane agent port (9443 by default). Check egress firewalls and the join token's validity.

**I expected a server to be created.** Custom never calls a cloud API. Create the machine yourself, then use Add a node to mint the join token and install command.

## FAQ

**When do I pick custom instead of a cloud provider?**

For bare metal, on-prem hardware or a VM at a provider Levelrail does not integrate with.

**Are Linode, Vultr or Scaleway provisioning supported?**

Not yet. They are planned; use a custom data center and manual enrollment until they land.

## Related

- [Hetzner Cloud](/integrations/cloud-providers/hetzner): Cloud node providers
- [AWS EC2](/integrations/cloud-providers/aws): Cloud node providers
- [All cloud node providers](/integrations/cloud-providers)
- [Custom data center documentation](https://docs.docker.com/engine/install/), maintained by the vendor
