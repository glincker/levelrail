---
title: "Use Microsoft Azure with Levelrail: node provisioning"
sidebarTitle: "Microsoft Azure"
description: "Let Levelrail create a server at Microsoft Azure with a first-boot script that installs Docker and enrolls the node agent."
---

Let Levelrail create a server at Microsoft Azure with a first-boot script that installs Docker and enrolls the node agent.

This page covers how to create servers and enroll them as nodes.

## What you get

- Create a server at Microsoft Azure from the Add node wizard or the CLI, with live region and size pickers.
- A cloud-init script that installs Docker if missing and starts the agent, which dials out to the control plane.
- Provision status from creating through booting and enrolling to ready, or failed with a reason.

## Set up

1. Prepare the credential. Choose a method: service_account_json (a service principal with a client secret) or workload_identity_federation (a federated token file instead of a secret).
2. In the dashboard, open **Settings > Cloud node providers** (`/settings/node-providers?connect=azure`), then Nodes > Add node.
3. Connect runs a read-only region listing first and stores nothing when it fails, so a saved connection already works.

From the CLI:

```bash
levelrail-cli nodes providers connect --provider azure --method service_account_json --provider-token '<ONE_LINE_JSON>'
levelrail-cli nodes provision --provider azure --region <REGION> --size <SIZE> --name <NODE_NAME>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Contributor on one resource group | The service principal creates a VNet, subnet, public IP, NIC and VM inside an existing group. |
| Reader at subscription scope | Region and size lookups call subscription APIs that a resource group role does not authorize. |

## Endpoint and format

Credential is one line of JSON with tenant_id, client_id, client_secret, subscription_id and resource_group. For federation, replace client_secret with federated_token_file.

## Security notes

- A saved credential is envelope encrypted and never shown again, in the API or the audit log.
- Piping a credential on stdin avoids leaving it in shell history; a flag value is visible in the process list.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The resource group must already exist; provisioning never creates it.
- Azure VMs get no SSH access, because the generated password is discarded. Configure access in the Azure console if you need it.
- Deletion of the NIC, public IP and OS disk relies on Azure's cascading delete and was not verified against a real subscription.
- Managed identity is not wired in yet.

## Troubleshooting

**Connect refuses to store the credential.** Connect runs a read-only region listing first and stores nothing when it fails. Also grant Reader on the subscription, or the region and size pickers fail even though VM creation works.

**A provision sits in enrolling.** The server booted but the agent has not dialed home. Read /var/log/cloud-init-output.log from the provider console; the provision is marked failed after APP_NODE_PROVISION_TIMEOUT (default 20 minutes).

**A name conflict returns 409.** The node name is already used by an enrolled node or an unfinished provision. Pick another name.

## FAQ

**Can I avoid a long lived client secret?**

Yes. Set federated_token_file to a JWT file from an OIDC issuer your app registration trusts. Levelrail does not mint the JWT, and the file is re-read when a fresh token is needed.

**Does Levelrail create the resource group?**

No. It must exist before you connect.

## Related

- [Azure Container Registry](/integrations/container-registries/acr): Container registries
- [Google Cloud](/integrations/cloud-providers/gcp): Cloud node providers
- [All cloud node providers](/integrations/cloud-providers)
- [Microsoft Azure documentation](https://learn.microsoft.com/azure/), maintained by the vendor
