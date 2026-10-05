---
description: Pick a cloud provider and add a provisioned node in three commands, with the credential each provider needs. Auth modes, cost and known gaps are on the node provisioning page.
---

# Multi-cloud provisioning quickstart

[Node provisioning](/node-provisioning) explains how cloud provisioning works: the join-token flow it drives, the cloud-init script, status polling and the known gaps per provider. This page is the short version: which providers are supported, what credential each needs, and the fastest path to a running node.

## Providers at a glance

| Provider | Credential | Needed before the first server |
| --- | --- | --- |
| Hetzner Cloud | Project API token | Nothing else |
| DigitalOcean | Personal access token | Nothing else |
| AWS EC2 | Access key and secret, an assumable role ARN, or the control plane's own AWS identity | A default VPC in the target region |
| Azure | Service principal JSON, or workload identity federation | An existing resource group |
| GCP | Service account JSON key | A project with the `default` VPC network |

GCP has no federation mode. The reason, and the three AWS credential modes, are in [node provisioning](/node-provisioning#provider-credentials).

::: warning Servers are not deleted for you
Every provider bills by the hour while the server exists. `levelrail-cli nodes delete` removes the node record, not the VM, so delete the server at the provider too. See [cost expectations](/node-provisioning#cost).
:::

## Add a node

<Steps>
<Step title="Store the provider credential">

Once per provider. Paste or pipe the credential when prompted, so it stays out of shell history.

```bash
levelrail-cli nodes providers set-credential --provider hetzner
```

</Step>
<Step title="Pick a region and size">

List what your account offers rather than guessing. The ids in the next step (`fsn1`, `cx22`) are Hetzner examples only.

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://control-plane.example.com/api/v1/node-providers/hetzner/regions"
curl -H "Authorization: Bearer $TOKEN" \
  "https://control-plane.example.com/api/v1/node-providers/hetzner/sizes?region=fsn1"
```

The dashboard's **Add node** wizard lists both live.

</Step>
<Step title="Provision and wait for ready">

```bash
levelrail-cli nodes provision --provider hetzner --region fsn1 --size cx22 --name build-1
levelrail-cli nodes provisions show <id>
```

Repeat `show` until the status is `ready`. Status moves through `creating`, `booting` and `enrolling`. Add `--role build` to make it a build node.

</Step>
</Steps>

The same shape works for DigitalOcean, AWS, Azure and GCP: store the credential with `--provider <name>`, then provision. AWS, Azure and GCP fold account detail (key and secret or role ARN, resource group, service account) into the credential itself. See [required token scopes](/node-provisioning#provider-credentials).

## Next steps

<CardGroup :cols="2">
<Card title="Node provisioning" href="/node-provisioning">

How it works, auth modes, cost and known gaps.

</Card>
<Card title="Multi-node" href="/multi-node">

Placement, drain, certificates and the mesh once the node is online.

</Card>
</CardGroup>
