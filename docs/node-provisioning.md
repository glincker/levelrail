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

AWS, Azure and GCP are not supported yet. This is a known, explicit gap
(tracked as a follow-up), not an oversight: both supported providers here
have a small, single-token REST API and a widely used, cheap VM tier that
matches this product's own 1-10 machine target audience. The three big
clouds each need materially more setup (IAM roles, VPC/network config,
image selection across regions) before a "paste a token, get a server"
flow is honest.

## Required token scopes

- **Hetzner**: a project API token with **Read & Write** access. Hetzner
  tokens are scoped to one project; provisioning creates servers in
  whichever project the token belongs to.
- **DigitalOcean**: a personal access token with **read and write** scope.

Store the token once, under Settings -> Cloud node providers
(`/settings/node-providers`), or via `nodes providers set-credential`. It is
encrypted at rest the same way every other integration credential in this
platform is (envelope encryption, ADR 010) and never echoed back once saved.

## Cost expectations

Both providers bill by the hour (or fractions of one) for however long the
server exists. Provisioning here never deletes a server on your behalf: if
a provision fails partway through, or you decide not to use a node anymore,
delete the server from the provider's own dashboard, or with
`levelrail-cli nodes delete <id>` once it has enrolled (that removes the
registry row, not the underlying VM; see that command's own known gap).
The cheapest size on either provider (roughly 3-5 EUR/USD a month as of
this writing) is enough for a `general` role node; a `build` role node
benefits from more CPU and memory since builds run there.

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

## What was not tested

This feature was built and reviewed without a real Hetzner or DigitalOcean
account available: the provider clients (`internal/provision`) are tested
against fake HTTP servers standing in for each API, not the real thing.
Before relying on this in production, provision one real node per provider
and confirm it reaches `ready` end to end.
