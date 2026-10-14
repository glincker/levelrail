---
title: "Use Google Cloud with Levelrail: node provisioning"
sidebarTitle: "Google Cloud"
description: "Let Levelrail create a server at Google Cloud with a first-boot script that installs Docker and enrolls the node agent."
---

Let Levelrail create a server at Google Cloud with a first-boot script that installs Docker and enrolls the node agent.

This page covers how to create servers and enroll them as nodes.

## What you get

- Create a server at Google Cloud from the Add node wizard or the CLI, with live region and size pickers.
- A cloud-init script that installs Docker if missing and starts the agent, which dials out to the control plane.
- Provision status from creating through booting and enrolling to ready, or failed with a reason.

## Set up

1. Prepare the credential. Use the service_account_json method: a service account key with Compute Instance Admin on the project.
2. In the dashboard, open **Settings > Cloud node providers** (`/settings/node-providers?connect=gcp`), then Nodes > Add node.
3. Connect runs a read-only region listing first and stores nothing when it fails, so a saved connection already works.

From the CLI:

```bash
levelrail-cli nodes providers connect --provider gcp --method service_account_json --provider-token '<KEY_JSON>'
levelrail-cli nodes provision --provider gcp --region <REGION> --size <SIZE> --name <NODE_NAME>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Compute Instance Admin on the project | Lets the service account create, get and delete instances. |
| compute.regions.list and compute.machineTypes.list | Used by the region and size pickers. |

## Endpoint and format

The service account JSON key, pasted as is. The project id comes from the key.

## Security notes

- A saved credential is envelope encrypted and never shown again, in the API or the audit log.
- Piping a credential on stdin avoids leaving it in shell history; a flag value is visible in the process list.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The project ID is read from the key's own project_id field.
- A VPC network named default must exist in the project.
- There is no workload identity federation mode for GCP.
- An external IPv4 address is billed next to the VM.

## Troubleshooting

**Connect refuses to store the credential.** Connect runs a read-only region listing first and stores nothing when it fails. Paste the service account JSON key as is, and confirm the project has a network named default.

**A provision sits in enrolling.** The server booted but the agent has not dialed home. Read /var/log/cloud-init-output.log from the provider console; the provision is marked failed after APP_NODE_PROVISION_TIMEOUT (default 20 minutes).

**A name conflict returns 409.** The node name is already used by an enrolled node or an unfinished provision. Pick another name.

## FAQ

**Does deleting the node delete the server at Google Cloud?**

No. `nodes delete` removes the node record only. Delete the server at the provider, because provisioning never removes servers for you.

**Which port does the new server need open?**

None inbound. The agent dials out to the control plane agent port (9443 by default) and uses outbound HTTPS for image pulls.

## Related

- [Google Cloud Storage](/integrations/backup-destinations/gcs): Backup destinations
- [Google Artifact Registry](/integrations/container-registries/gar): Container registries
- [All cloud node providers](/integrations/cloud-providers)
- [Google Cloud documentation](https://cloud.google.com/compute/docs), maintained by the vendor
