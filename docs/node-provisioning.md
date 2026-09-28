---
description: Automate creating a new server at a cloud provider and enrolling it as a node, instead of running the join flow on a box you provisioned by hand.
---

# Node provisioning

[Multi-node](/multi-node) covers enrolling a machine you already have. This
page covers automating the part before that: creating the machine itself at
a cloud provider.

Everything here builds on the same join-token enrollment flow multi-node
already uses. Provisioning does not replace it, it drives it: mint a token,
hand it to a fresh cloud VM's first-boot script, and let the agent enroll
the same way it always does.

## Supported providers

- **Hetzner Cloud** (`https://docs.hetzner.cloud`)
- **DigitalOcean** (`https://docs.digitalocean.com/reference/api/`)
- **Azure** (`https://learn.microsoft.com/en-us/rest/api/compute/`)
- **GCP** (`https://cloud.google.com/compute/docs/reference/rest/v1`)

AWS is not supported yet. Hetzner and DigitalOcean each have a small,
single-token REST API; Azure and GCP need materially more setup (a service
principal or a service account, a resource group or project, network
resources created alongside the VM) before a "paste a credential, get a
server" flow works, covered under Required token scopes below.

## Required token scopes

- **Hetzner**: a project API token with **Read & Write** access. Hetzner
  tokens are scoped to one project; provisioning creates servers in
  whichever project the token belongs to.
- **DigitalOcean**: a personal access token with **read and write** scope.
- **Azure**: a service principal (Azure AD app registration) with
  **Contributor** access on one resource group, entered as a single-line
  JSON object: `{"tenant_id","client_id","client_secret","subscription_id","resource_group"}`.
  The resource group must already exist; provisioning creates a VNet,
  subnet, public IP, NIC and VM inside it, but never the group itself.
- **GCP**: a service account JSON key with the **Compute Instance Admin**
  role on the project, pasted as-is (minified to one line). The project ID
  is read from the key's own `project_id` field; there is no separate
  project setting. A default VPC network named `default` must already
  exist in the project (every new GCP project has one unless it was
  explicitly deleted).

Store the credential once, under Settings -> Cloud node providers
(`/settings/node-providers`), or via `nodes providers set-credential`. It is
encrypted at rest the same way every other integration credential in this
platform is (envelope encryption, ADR 010) and never echoed back once saved.

The CLI's `--provider-token` flag is accepted for scripting, but its value
ends up in shell history and the process list while the command runs.
Prefer piping the credential instead (`echo "$TOKEN" | levelrail-cli nodes
providers set-credential --provider hetzner`), or omit the flag entirely
and run interactively for a no-echo prompt.

## Cost expectations

Every provider bills by the hour (or fractions of one) for however long the
server exists. Provisioning here never deletes a server on your behalf: if
a provision fails partway through, or you decide not to use a node anymore,
delete the server from the provider's own dashboard, or with
`levelrail-cli nodes delete <id>` once it has enrolled (that removes the
registry row, not the underlying VM; see that command's own known gap).
The cheapest size on any of the four providers (roughly 3-7 EUR/USD a month
as of this writing) is enough for a `general` role node; a `build` role
node benefits from more CPU and memory since builds run there.

Azure and GCP also bill for the small networking resources provisioning
creates alongside the VM (a public IP on Azure, none extra on GCP, whose
external IP is ephemeral and free while attached to a running instance).
These are pennies a month, not a meaningful addition to the VM's own cost.

## How it works

1. `POST /api/v1/nodes/provision` (or `nodes provision` / the Nodes page's
   "Add node" wizard) mints a join token through the exact same code path
   `POST /api/v1/nodes/join-tokens` uses, then calls the provider's API to
   create a server with a cloud-init script as its user-data.
2. The cloud-init script installs Docker if the image doesn't already have
   it (the same `get.docker.com` convenience script `install.sh` uses),
   then starts the node agent as a systemd-managed container, with the
   join token and CA fingerprint passed through a root-only environment
   file rather than the container's command line.
3. `GET /api/v1/node-provisions/{id}` (or `nodes provisions show <id>`, or
   the wizard's own progress view) recomputes status live on every call:
   it asks the provider whether the server is up, and checks whether a
   node with the expected name has enrolled yet. Status moves through
   `creating` -> `booting` -> `enrolling` -> `ready`, or `failed` with a
   reason.
4. Once that node is found, its accepted workload kinds are set to match
   the `role` the provision was created with (`general`:
   `accepts_app_workloads=true`, `build`: `accepts_build_workloads=true`):
   enrollment itself always starts a node as a plain app node, with no way
   to carry an operator's chosen role through the join-token exchange, so
   this is the first point after enrollment this feature controls.

A name already used by an enrolled node, or by another provision that
hasn't failed, is rejected up front (409): the same name is how step 3
recognizes which node belongs to which provision, and reusing one would
let a provision report ready against an unrelated, pre-existing VM.

### Why there's no "installing" stage in practice

The status vocabulary includes `installing` for a future, finer-grained
signal, but nothing emits it today: this control plane has no way to see
inside the VM while cloud-init runs. Once the provider reports the server
as up, everything between "just booted" and "successfully dialed home" is
reported as a single `enrolling` stage. If a provision sits at `enrolling`
for an unusually long time, check the server's own console output at the
provider (cloud-init logs to `/var/log/cloud-init-output.log`) before
assuming it's stuck; `APP_NODE_PROVISION_TIMEOUT` (default 20 minutes)
eventually marks it `failed` on its own either way.

### The agent image, not a downloaded binary

Unlike the control plane binary `install.sh` installs (verified against a
published `checksums.txt`), there is currently no raw `levelrail-agent`
binary release to download and check a checksum against: the agent ships
only as a container image (`ghcr.io/glincker/levelrail-agent`, one tag per
release, cosign-signed by the release pipeline). Cloud-init therefore pulls
and runs that image rather than curling a binary. The pull itself is a
plain `docker pull` over the registry's own TLS; cloud-init does not
additionally verify the image's cosign signature before running it. That
is a real, known gap in this specific automation path: treat it the same
way you'd treat any other unverified script executed on first boot, until
a raw binary release (or an automated signature check in the cloud-init
script) closes it.

### The join token and CA fingerprint are visible on the node

They're passed to the agent container as environment variables. `docker
inspect levelrail-agent` on the node itself will show them for as long as
the container exists. This is the same exposure any container's env vars
have; it's not specific to provisioning. Treat a provisioned node as
trusted infrastructure, the same way you would a manually enrolled one.

## Known limitations

- **Closing the wizard's progress view stops the browser from tracking
  that provision.** The server keeps provisioning either way (nothing
  server-side is cancelled), and `nodes provisions show <id>` or `GET
  /api/v1/node-provisions/{id}` still work; there is just no UI screen
  yet to reopen and watch it from. Use the CLI or the API meanwhile.
- **Azure VMs get no SSH access by default.** Azure's VM creation API
  requires a password when no SSH public key is supplied, and there is no
  credential input for one today; a random, unrecoverable throwaway
  password is generated internally purely to satisfy that requirement,
  then discarded. This is not a regression: the agent enrolls by dialing
  out to the control plane, never over SSH, matching every other
  provider's own "no inbound ports" default. An operator who wants SSH
  access to an Azure-provisioned node has to configure it separately
  through the Azure console.
- **Azure's DeleteServer relies on cascading resource deletion.** The VM's
  NIC, public IP and OS disk are all created with `deleteOption: "Delete"`,
  which Azure documents as cascading their removal from a single VM delete
  call. This was not verified against a real subscription (see below); if
  it doesn't behave as documented, a deleted Azure node can leave a NIC and
  public IP behind, billable until removed by hand from the Azure console.
- **Azure and GCP catalogs have no pricing.** `ListSizes` returns an empty
  `price_monthly` for both: Azure's retail pricing and GCP's billing
  catalog are both separate APIs this integration doesn't call, unlike
  Hetzner and DigitalOcean, which return a size's price inline.

## What was not tested

This feature was built and reviewed without real cloud accounts available
for any of the four providers: the provider clients
(`internal/provision`) are tested against fake HTTP servers standing in
for each API, not the real thing. Azure and GCP in particular were not
exercised against a real subscription or project at all, so beyond the
individual gaps called out above, the entire multi-step resource chain
(resource group, VNet/subnet, public IP, NIC, VM for Azure; the OAuth2
JWT bearer token exchange and instance creation for GCP) is unverified
end to end. Before relying on this in production, provision one real
node per provider and confirm it reaches `ready` end to end.
