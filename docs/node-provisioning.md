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
- **AWS EC2** (`https://docs.aws.amazon.com/ec2/`)

Azure and GCP are not supported yet. This is a known, explicit gap
(tracked as a follow-up), not an oversight: Hetzner and DigitalOcean have
a small, single-token REST API and a widely used, cheap VM tier that
matches this product's own 1-10 machine target audience. AWS needed
materially more setup to support honestly (IAM credentials, VPC/subnet
discovery, AMI lookup, a curated instance type list instead of EC2's full
catalog, see below); Azure and GCP would need the same treatment again.

## Required token scopes

- **Hetzner**: a project API token with **Read & Write** access. Hetzner
  tokens are scoped to one project; provisioning creates servers in
  whichever project the token belongs to.
- **DigitalOcean**: a personal access token with **read and write** scope.
- **AWS**: see "AWS credentials" below.

Store the token once, under Settings -> Cloud node providers
(`/settings/node-providers`), or via `nodes providers set-credential`. It is
encrypted at rest the same way every other integration credential in this
platform is (envelope encryption, ADR 010) and never echoed back once saved.

The CLI's `--provider-token` flag is accepted for scripting, but its value
ends up in shell history and the process list while the command runs.
Prefer piping the token instead (`echo "$TOKEN" | levelrail-cli nodes
providers set-credential --provider hetzner`), or omit the flag entirely
and run interactively for a no-echo prompt.

## AWS credentials

EC2 has no single bearer token the way Hetzner and DigitalOcean do.
`nodes providers set-credential --provider aws` (or the Settings page)
takes one of two credential shapes, both stored under the same encrypted
credential slot the other providers use:

- **Access key and secret** (`--provider-token` as the access key id,
  `--secret-access-key` for the secret). Optional `--session-token` for
  temporary credentials, `--region` to set the default region, and
  `--role-arn` to assume an IAM role via STS before every call: with a
  role set, the stored key only needs `sts:AssumeRole` on that one role,
  not direct EC2 permissions.
- **This control plane's own AWS identity** (`--use-ambient-credentials`):
  resolves credentials from the environment, shared config, or an EC2
  instance profile instead of a stored key. Only useful when the control
  plane itself runs on AWS. Combine with `--role-arn` to still narrow the
  effective permissions via STS.

Full OIDC federation (`AssumeRoleWithWebIdentity`, the way GitHub Actions
authenticates to AWS) is not implemented: it requires this control plane
to be its own trusted OIDC token issuer, which does not exist yet. The
role-ARN and ambient-credential paths above are the supported ways to
avoid a long-lived key with direct EC2 permissions.

A minimal least-privilege IAM policy for the static-key path:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "ec2:RunInstances",
      "ec2:TerminateInstances",
      "ec2:CreateSecurityGroup",
      "ec2:CreateTags",
      "ec2:Describe*"
    ],
    "Resource": "*"
  }]
}
```

`ec2:Describe*` covers the regions, instance types, images, VPCs, subnets
and security groups provisioning looks up; a tighter policy can narrow it
to the specific `Describe*` actions above if you'd rather enumerate them.
`RunInstances`/`CreateSecurityGroup`/`CreateTags` need broader `Resource`
scoping in AWS's own IAM model than a single ARN can express for a
not-yet-created instance, so this policy is not resource-scoped further.

AWS provisioning currently requires the target region to have a **default
VPC** (true for every AWS account created since December 2013, unless it
was explicitly deleted): CreateOpts, shared across every provider this
platform supports, has no field yet to name a specific VPC or subnet.
Every AWS-provisioned instance shares one security group per control
plane (created on first use, reused after), with no inbound rules at all,
matching this platform's "no inbound ports on managed servers"
architecture; optional SSH inbound for manual access is not wired through
yet. `ListSizes` for AWS returns a small curated list of common `t3.*`
instance types rather than EC2's full catalog, the same reasoning
Hetzner/DigitalOcean's own short size lists already follow.

## Cost expectations

Every provider bills by the hour (or fractions of one) for however long
the server exists. Provisioning here never deletes a server on your
behalf: if a provision fails partway through, or you decide not to use a
node anymore, delete the server from the provider's own dashboard, or
with `levelrail-cli nodes delete <id>` once it has enrolled (that removes
the registry row, not the underlying VM; see that command's own known
gap). The cheapest size on Hetzner or DigitalOcean (roughly 3-5 EUR/USD a
month as of this writing) is enough for a `general` role node; AWS's
cheapest curated size (`t3.micro`) is comparable. A `build` role node
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

## What was not tested

This feature was built and reviewed without a real Hetzner, DigitalOcean
or AWS account available: the Hetzner/DigitalOcean clients
(`internal/provision`) are tested against fake HTTP servers standing in
for each API, and the AWS client against a hand-written fake implementing
the same narrow interface the real EC2 SDK client does, not the real
thing. Before relying on this in production, provision one real node per
provider and confirm it reaches `ready` end to end. AWS in particular has
not been exercised against a real account at all: the default-VPC lookup,
AMI resolution, security group creation, and the STS-assume-role and
ambient-credential paths are all only as correct as the fake responses
they were tested against.
