# ADR 068: Oracle Cloud provisioning driver

Status: Decided (do not build now; document the manual path)

Date: 2026-11-01

## Context

Oracle Cloud's Always Free tier includes Ampere A1 (arm64) capacity, which
makes it the most likely host for a zero-cost Levelrail install. Users ask
whether Levelrail can create that VM for them, as it does elsewhere.

What exists today, checked in code:

- `internal/provision/provisioner.go` defines `Provisioner` (list regions,
  list sizes, create, get, delete). Hetzner, DigitalOcean, AWS, Azure and GCP
  implement it over plain net/http with no cloud SDK.
- `internal/provision/cloudinit.go` (`RenderCloudInit`) renders the
  cloud-init that installs the agent and joins it with a pinned CA
  fingerprint. It is provider-neutral.
- Oracle appears only in detection: `internal/hostprovider/provider.go`
  recognises an Oracle host by DMI text or the instance metadata endpoint, and
  `internal/hostprovider/guidance.go` tells the operator to open ports in the
  security list and in the host firewall, because Oracle images ship an
  iptables rule set that rejects other ports.
- Provider credentials are stored per provider through
  `internal/api/node_provider_connect.go`, with method inference per provider.

What a driver would need:

- Request signing: Oracle's API uses signed HTTP requests with an RSA API key
  (tenancy id, user id, key fingerprint, private key, region). This can be
  written with the standard library, so no `go.mod` change, but it is a new
  signing scheme to own and test.
- Create: compartment pick, VCN and subnet (or reuse), an ingress rule set for
  TCP 80 and 443 and UDP 51820 (mesh), an A1 Flex instance with OCPU and
  memory, an arm64 image, and user data from `RenderCloudInit` plus an
  iptables step for the image's host rules.
- Guards: Always Free A1 is a tenancy-wide allowance, not a per-instance
  flag. The API does not mark a create as free or billable. Oracle documents
  that idle Always Free instances can be reclaimed when CPU, network and (for
  A1) memory stay low for a week.

## Decision

Do not build the driver now. Document the manual path instead, and revisit
when the conditions below are met.

Reasons:

1. **The driver cannot promise "free".** Whether a create stays inside the
   free allowance depends on everything else in the tenancy and on the account
   type. A driver that says "free" and is wrong costs the user money. A driver
   that cannot say it adds little over the console wizard.
2. **The risky part is account setup, not the VM.** Quota policy, budget alert,
   compartment and API key creation are tenancy administration. A provisioning
   driver should not create them, and without them the user has no guard.
3. **Idle reclaim is outside our control.** A small home-lab install is
   exactly the low-usage profile that can be reclaimed. Automating creation
   makes that loss easier to walk into. Keep-alive load is not an acceptable
   workaround.
4. **Cost to own.** A new signing scheme, network objects and image lookup is
   about a week of work plus upkeep, for a path that runs once per user.
5. **Manual path is short.** Console create, open ports, run the install
   script. The join flow and cloud-init already work on any Linux host.

What ships instead (docs only):

- An Oracle page under docs with: account guards first (a quota policy that
  caps A1 OCPU and memory and a budget alert, both created by the user in the
  console), instance create with an A1 Flex shape inside the user's own
  allowance, security list rules for TCP 80, 443 and UDP 51820, the host
  iptables step (matching `guidance.go`), then the installer. It states what
  was run and when, not a quota number, because published allowances differ
  between sources.
- A note on idle reclaim and a pointer to off-host backups.
- The existing host detection guidance stays the in-product hint.

Revisit when all hold: the zero-cost tutorial has a real-run ledger on Oracle,
users ask for automated create more than once, and a guard design exists that
refuses a create the driver cannot prove is inside the user's stated limits.

## Design sketch, if built later

Recorded so a later ADR starts from it, not as approval.

- `internal/provision/oracle.go` implementing `Provisioner` with standard
  library request signing.
- Credentials: tenancy id, user id, fingerprint and private key PEM stored
  through `internal/secrets`; the key is added to the `secretmask` registry and
  never returned. Region and compartment are not secret.
- Guard before create: refuse unless requested OCPU and memory plus the sum of
  existing A1 instances in the compartment stay within
  `APP_ORACLE_MAX_A1_OCPU` and `APP_ORACLE_MAX_A1_MEMORY_GB` (operator-set, no
  built-in allowance figure), and unless a quota policy is found, or the
  operator confirms a billable create explicitly.
- Network: reuse a VCN when tagged as ours, else create one with only the three
  ingress rules; never widen an existing security list.
- Delete removes only objects the driver tagged.

## Alternatives rejected

- **Build it now with a "free" label.** Rejected for reason 1.
- **Terraform or a cloud SDK.** `go.mod` growth and a second toolchain for one
  provider; owner decision, not recommended.
- **Ship a cloud-init file only.** Folded into the docs page as an optional
  user-data block, generated by the existing renderer, not a new feature.

## Security

- Docs never show real OCIDs, IPs or keys; examples use placeholders.
- The docs page tells users to create an API key for a user with a narrow
  policy, not the tenancy administrator, if they script anything themselves.
- No new code path, so no new secret handling in this decision.

## Failure modes (of the documented path)

- Ports open in the security list but blocked by host iptables: the existing
  detection guidance and doctor catch it; the docs page repeats it.
- Capacity errors on A1 create in a region: docs say to retry later or pick
  another availability domain; nothing to automate.
- Instance reclaimed for idleness: backups off the VM are the recovery.

## What cannot be verified locally

- The current free A1 allowance and account-type rules.
- Default host firewall rules on each Oracle image.
- Reclaim behaviour on a real low-usage install.

All three need the owner's test tenancy and are checked during the tutorial run.

## Testing plan

- Docs: two-pass real run (write, then follow verbatim on a fresh instance),
  with a dated ledger and placeholders in anything public.
- No code tests, since no code ships.

## Slices

| # | Slice | Files it owns | Effort | Model | Real host | Secret-sensitive |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Oracle page: guards, create, ports, iptables, install, reclaim note | new `docs/oracle-cloud.md`, `docs/public/llms.txt` | 0.5 day | Sonnet | Yes (run) | No (placeholders only) |
| 2 | Link from multi-cloud provisioning and the host detection guidance text | `docs/multi-cloud-provisioning.md`, `internal/hostprovider/guidance.go` (link only) | 0.25 day | Haiku | No | No |
| 3 | Second pass: follow the page verbatim on a fresh instance, fix gaps | `docs/oracle-cloud.md` | 0.5 day | Sonnet | Yes | No |

No env settings in this decision. The sketch above names the env a future
driver would read.

## Consequences

- No new provider code or credential type.
- Users do one manual create, with the guards in front of them.
- The decision can be reopened by a new ADR when the revisit conditions hold.

## Open questions

- Is the owner's test tenancy close enough to a strict Always Free account to
  document from, or is a second, strict account needed for the run?
- Should the docs page include a generated user-data block, or only the
  installer command?
