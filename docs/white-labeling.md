---
description: How the brand layer works, which names and prefixes it drives (dashboard, agent unit, firewall rule tag, mesh interface, update repository), the brand.yaml fields and APP_BRAND_* overrides, and what a fork must still change by hand.
---

# White-labeling

The product name is never hardcoded in the Go or web source. Every user-visible identity string comes from one `Brand` struct in `internal/brand`, loaded at startup from a `brand.yaml` file and optionally overridden by environment variables. This page explains what that layer controls so you can run a renamed fork, or just understand why the platform names things the way it does.

<InlineToc default-open />

## How the brand is loaded

1. The control plane reads `brand.yaml` from the path in `APP_BRAND_FILE`, or `./brand.yaml` relative to its working directory when that is unset. The installer's systemd unit runs in the data directory, so the copy `install.sh` writes there is the one that is read.
2. Any `APP_BRAND_*` environment variable that is set and non-empty overrides the matching file field. Env wins over the file, so a deployment can be rebranded without editing a file or rebuilding.
3. `primary_color_dark` falls back to `primary_color` when empty.
4. Startup fails if `name` or `binary_name` is empty, or if the file cannot be read or parsed. Running the binary directly without a `brand.yaml` produces an error telling you to supply one or set `APP_BRAND_FILE`.

The node agent reads the same file and overrides to derive the same short name the control plane uses (see [what the brand drives](#what-the-brand-drives)). The dashboard does not embed any name: it fetches `GET /api/v1/brand` on boot, which is reachable before sign-in so the login screen can be branded, and applies the colors as CSS variables.

The env prefix is deliberately `APP_BRAND_`, not one containing the product name, so a rename never needs an env migration.

## Fields

Each field maps to one key in `brand.yaml` and one env var.

| `brand.yaml` key | Env var | Purpose |
| --- | --- | --- |
| `name` | `APP_BRAND_NAME` | Display name. Required. Used in emails (invites, password reset), the two-factor authenticator label, the passkey relying-party name, the GitHub App display name, and the agent unit description. |
| `short_name` | `APP_BRAND_SHORT_NAME` | Namespace stem for Docker networks, resources and prefixes. Lowercased where a prefix is built. |
| `binary_name` | `APP_BRAND_BINARY_NAME` | Control plane binary name. Required. Drives the agent binary name, the CLI name shown by diagnostics, and app spec file discovery (see below). |
| `domain` | `APP_BRAND_DOMAIN` | Product domain string. |
| `support_url` | `APP_BRAND_SUPPORT_URL` | Support destination, for example an issue tracker. Also the fallback web push contact when no support email is set. |
| `support_email` | `APP_BRAND_SUPPORT_EMAIL` | Direct contact address. Used as the web push subscriber contact. Optional. |
| `primary_color` | `APP_BRAND_PRIMARY_COLOR` | Accent color, applied as a CSS variable in the dashboard. |
| `primary_color_dark` | `APP_BRAND_PRIMARY_COLOR_DARK` | Accent color in dark mode. Falls back to `primary_color`. |
| `logo_svg` | `APP_BRAND_LOGO_SVG` | Inline SVG logo markup. |
| `docs_url` | `APP_BRAND_DOCS_URL` | Documentation root, used for links to guides. |
| `discussions_url` | `APP_BRAND_DISCUSSIONS_URL` | Community destination. Optional. |
| `repo_url` | `APP_BRAND_REPO_URL` | Source repository root. Also decides where release checks look (see below). |

The optional contact fields follow one rule: when empty, the corresponding link is not rendered.

`APP_BRAND_FILE` is not a brand field. It only selects which file to load.

## What the brand drives

| Derived value | Rule | Used for |
| --- | --- | --- |
| Agent name | lowercased `short_name` plus `-agent` | The node agent's systemd unit, container name, environment file, data directory stem and default image name on provisioned nodes. |
| Firewall rule tag | lowercased `short_name` plus `:` | The comment prefix on every host `ufw` rule the platform creates, so it only ever removes its own rules. See [Host firewall](host-firewall.md). |
| Mesh interface | `short_name` lowercased, letters and digits only, plus `0`, at most 15 characters; `wg0` if nothing usable remains | The WireGuard interface name on every node. The agent and control plane derive it from the same file, so they agree. |
| Update repository | `owner/name` parsed from `repo_url`, lowercased | Where the control plane checks for new releases, version skew and the self-upgrade channels. An empty or malformed `repo_url` yields no slug, so no release lookups. |
| Agent binary name | `binary_name` plus `-agent` | The agent binary named in the join token response when enrolling a node. |
| Diagnostics CLI name | `binary_name` plus `-cli` | The command named in `doctor` fix hints. |
| App spec filenames | lowercased `binary_name` plus `.yaml` or `.yml` | One of the filenames discovered in a repo, after `app.yaml`, `app.yml`, `deploy.yaml` and `deploy.yml`. |
| Docker namespace | `short_name` | Network and resource name prefix for app teardown, deploy previews, supply-chain scans, model resources and the syslog log-drain sender. |
| Badge label | `short_name` plus ` deploy` | The text on the [deploy status badge](deploy-status-badge.md). |

The CLI takes its command name from `os.Args[0]`, so renaming the `*-cli` binary renames the command in every usage message and in generated shell completions.

## Building a renamed binary

The binary file name comes from how you build and install it, not from `brand.yaml`. The release workflow builds `levelrail`, `levelrail-cli` and `levelrail-agent` and injects only the version, through `-ldflags "-X github.com/GLINCKER/levelrail/internal/version.Version=..."`. Set `binary_name` in `brand.yaml` to match whatever you name the control plane binary, so the derived agent and CLI names line up with the files you ship.

## What a fork must still change

The brand layer covers names and prefixes. These are concrete locations that stay upstream until you change them:

- **Agent image registry.** Provisioned nodes pull `ghcr.io/glincker/<agent-name>:<tag>`. The `ghcr.io/glincker/` owner is a constant in `internal/provision/cloudinit.go` and in the SSH provisioning path in `internal/api/node_ssh_provision.go`, not a brand field. A fork publishing its own agent image must change both.
- **Go module path.** The module is `github.com/GLINCKER/levelrail`. Renaming it means rewriting imports, and the [shared kit](kit.md) has its own module path.
- **The installer.** `install.sh` writes a default `brand.yaml` into the data directory only when none exists, and that default carries the upstream values. Edit the script's `write_brand` step, or place your own `brand.yaml` in the data directory before the first run.
- **Release and docs hosting.** `repo_url` and `docs_url` point where you tell them to, but your fork needs its own releases and its own docs site for those links to resolve.
- **Documentation text.** These docs name the product directly. The brand layer does not rewrite prose.

## Set a brand at runtime

Override only what you need, in the control plane's environment file:

```bash
APP_BRAND_NAME="Acme Deploy"
APP_BRAND_PRIMARY_COLOR="#0a7a5a"
APP_BRAND_SUPPORT_EMAIL="platform@acme.example"
```

Restart the control plane after changing a brand value, then reload the dashboard. Because the agent derives the mesh interface name from the same inputs, set the same `APP_BRAND_*` values (or the same `brand.yaml`) wherever an agent runs, or its interface name will not match the control plane's.

<CardGroup :cols="2">
<Card title="Shared Go kit" href="/kit">

The brand-neutral Go packages the platform shares with other programs.

</Card>
<Card title="Architecture" href="/architecture">

How the control plane, agent and dashboard fit together.

</Card>
</CardGroup>
