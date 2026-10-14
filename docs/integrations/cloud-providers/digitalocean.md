---
title: "Use DigitalOcean with Levelrail: node provisioning"
sidebarTitle: "DigitalOcean"
description: "Let Levelrail create a server at DigitalOcean with a first-boot script that installs Docker and enrolls the node agent."
---

Let Levelrail create a server at DigitalOcean with a first-boot script that installs Docker and enrolls the node agent.

This page covers how to create servers and enroll them as nodes.

## What you get

- Create a server at DigitalOcean from the Add node wizard or the CLI, with live region and size pickers.
- A cloud-init script that installs Docker if missing and starts the agent, which dials out to the control plane.
- Provision status from creating through booting and enrolling to ready, or failed with a reason.

## Set up

1. Prepare the credential. Use the api_token method: one DigitalOcean personal access token.
2. In the dashboard, open **Settings > Cloud node providers** (`/settings/node-providers?connect=digitalocean`), then Nodes > Add node.
3. Connect runs a read-only region listing first and stores nothing when it fails, so a saved connection already works.

From the CLI:

```bash
levelrail-cli nodes providers connect --provider digitalocean --method api_token
levelrail-cli nodes provision --provider digitalocean --region <REGION> --size <SIZE> --name <NODE_NAME>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Personal access token | Read and write scope. Least privilege is droplet and ssh_key create, read and delete. |

## Endpoint and format

Credential kind: bearer token

## Security notes

- A saved credential is envelope encrypted and never shown again, in the API or the audit log.
- Piping a credential on stdin avoids leaving it in shell history; a flag value is visible in the process list.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The size picker shows a live monthly price from the DigitalOcean API.
- Billing is hourly until you delete the droplet.

## Troubleshooting

**Connect refuses to store the credential.** Connect runs a read-only region listing first and stores nothing when it fails. Use a personal access token with read and write scope.

**A provision sits in enrolling.** The server booted but the agent has not dialed home. Read /var/log/cloud-init-output.log from the provider console; the provision is marked failed after APP_NODE_PROVISION_TIMEOUT (default 20 minutes).

**A name conflict returns 409.** The node name is already used by an enrolled node or an unfinished provision. Pick another name.

## FAQ

**Does deleting the node delete the server at DigitalOcean?**

No. `nodes delete` removes the node record only. Delete the server at the provider, because provisioning never removes servers for you.

**Which port does the new server need open?**

None inbound. The agent dials out to the control plane agent port (9443 by default) and uses outbound HTTPS for image pulls.

## Related

- [DigitalOcean Spaces](/integrations/backup-destinations/spaces): Backup destinations
- [Hetzner Cloud](/integrations/cloud-providers/hetzner): Cloud node providers
- [All cloud node providers](/integrations/cloud-providers)
- [DigitalOcean documentation](https://docs.digitalocean.com/), maintained by the vendor
