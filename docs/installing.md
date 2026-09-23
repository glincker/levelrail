---
description: Requirements, then every supported way to get the Levelrail control plane running on Linux with install.sh, Docker, or source build.
---

# Installing Levelrail

Levelrail ships as two static Go binaries (`levelrail`, the control
plane, and `levelrail-agent`, the node agent) plus a CLI
(`levelrail-cli`). This page covers every supported way to get the
control plane running on a real Linux host, how to verify it worked,
and how to upgrade or remove it afterward.

## Requirements

Confirm these before you provision a server.

**Supported OS**

Linux, `amd64` or `arm64`. `install.sh` checks `uname -s` and exits on anything else, and requires `systemctl` since it writes a systemd unit. No specific distro is enforced beyond that, and Docker itself is installed automatically via `get.docker.com` if it's missing. Windows and non-Linux nodes aren't supported, by design (see the root `CLAUDE.md`'s non-goals).

**RAM, CPU, and disk**

Nothing in `install.sh` or the control plane checks a minimum. `levelrail-cli doctor` (`GET /api/v1/system/doctor`) checks Docker reachability, free disk space (warns below 1 GiB by default), and data-directory writability, not a memory or CPU floor, and no official minimum has been benchmarked or published either; that measurement is Phase 5 work per the [roadmap](roadmap.md).

As a practical starting point, not a hard requirement: 1 vCPU / 1 GB RAM / 10 GB disk is enough to boot Docker and the control plane on a small single-node instance. BuildKit builds and whatever apps you deploy need headroom of their own on top of that, so 2 vCPU / 2 GB RAM / 20 GB disk is more comfortable in practice. Scale up from there based on what you actually run.

**Ports**

- **Control-plane node:** `80/tcp` and `443/tcp`, inbound, reachable from the internet. Port 80 specifically is required for Let's Encrypt's HTTP-01 challenge during ACME issuance; both are what embedded Caddy binds for ingress, and `GET /api/v1/system/doctor` checks both are free to bind. See [Domains and ingress: firewall](domains-and-ingress.md#firewall-ports-80-and-443) if one is blocked.
- **Agent-only nodes** (additional servers added later via [Multi-node](multi-node.md)): none. The agent dials *out* to the control plane and the control plane never initiates a connection, so a managed node needs no inbound ports open at all.
- `install.sh` can configure `ufw` for you with `LEVELRAIL_CONFIGURE_UFW=1` (see the table below), or open `80/tcp` and `443/tcp` yourself via your cloud provider's firewall, `ufw`, or `iptables`.

## Option 1: install.sh (recommended)

```
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
```

This is the same script linked from the root [README](../README.md). It handles the full setup automatically:

- Downloads the right `linux/amd64` or `linux/arm64` binary from the [latest GitHub release](https://github.com/glincker/levelrail/releases)
- Verifies the checksum
- Installs Docker via `get.docker.com` if missing
- Writes a `levelrail.service` systemd unit
- Starts the service
- Waits up to 60 seconds for the control plane to answer `GET /api/v1/brand`

Requires `curl`, `systemd`, and root access.

::: details Optional environment variables

The `install.sh` script reads these environment variables, all optional:

| Variable | Default | What it does |
| --- | --- | --- |
| `LEVELRAIL_VERSION` | latest release | Pin a specific release tag instead of resolving the newest one |
| `LEVELRAIL_INSTALL_DIR` | `/usr/local/bin` | Where the `levelrail` binary is installed |
| `LEVELRAIL_DATA_DIR` | `/var/lib/levelrail-data` | Control plane data directory (SQLite database, generated `brand.yaml`) |
| `LEVELRAIL_CONFIGURE_UFW` | unset (off) | Set to `1` to have the script configure `ufw`: allow SSH, then 80/443, then enable it if it wasn't already active. Off by default, the script never touches your firewall otherwise. |

:::

Common scenarios:

::: code-group
```bash [Pin a specific release]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.1.0 sh
```

```bash [Custom install directory]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_INSTALL_DIR=/opt/levelrail/bin sh
```

```bash [Custom data directory]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_DATA_DIR=/data/levelrail sh
```

```bash [Configure UFW automatically]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_CONFIGURE_UFW=1 sh
```

```bash [Multiple overrides]
sudo LEVELRAIL_VERSION=v0.1.0 LEVELRAIL_DATA_DIR=/data/levelrail sh
```
:::


## Option 2: Docker

Already running everything else as containers? See [docs/docker.md](docker.md) for `docker run` and `docker-compose.yml` examples.

Images available:
- `ghcr.io/glincker/levelrail` (control plane)
- `ghcr.io/glincker/levelrail-agent` (node agent)

Both are published for `linux/amd64` and `linux/arm64`, multi-arch, under three tag channels:

| Tag | Built from | Stability |
| --- | --- | --- |
| `:latest`, `:vX.Y`, `:vX.Y.Z` | a non-prerelease tag (`v1.2.3`) | stable release |
| `:beta` | a prerelease tag (`v1.2.3-beta.1`, `-rc.1`, etc.) | prerelease |
| `:edge` | every push to `main` | unreleased, use for testing only |

`:latest` and `:vX.Y` only ever move on a stable tag; `:beta` and `:edge` move continuously, so pin an exact `:vX.Y.Z` tag for anything you care about staying still.

### Verifying image signatures

Every image is signed keylessly with [cosign](https://docs.sigstore.dev/cosign/overview/) via GitHub Actions OIDC (no long-lived signing key), with an SBOM attached as a signed attestation. Verify a pulled image against this repository's release workflow:

```bash
cosign verify ghcr.io/glincker/levelrail:latest \
  --certificate-identity-regexp 'https://github.com/glincker/levelrail/\.github/workflows/release\.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Swap in `levelrail-agent` and any tag from the table above. To inspect the attached SBOM:

```bash
cosign download sbom ghcr.io/glincker/levelrail:latest
```

::: tip
`install.sh` remains the recommended path for a real single-node deployment since it also provisions Docker and a systemd unit for you. The Docker image is for operators who want Levelrail to fit into an existing container-only setup.
:::

## Option 3: build from source

See [Getting started: building from source](getting-started.md#build-the-binaries)
for the `go build` commands. This is the path for contributors and anyone
who wants to run an unreleased commit rather than a tagged version.

## Verifying the install

`install.sh` already fails loudly if the control plane doesn't come up healthy within 60 seconds. You can re-check the installation at any time:

**Quick checks (unauthenticated):**

```bash
# Health check the control plane responds
curl -fsS http://127.0.0.1:8080/api/v1/brand

# Check systemd service status
systemctl status levelrail

# View logs in real time
journalctl -u levelrail -f
```

**Authenticated checks (after initial setup):**

Once you've created an admin account and minted an API token (see [docs/getting-started.md](getting-started.md#deploy-your-first-app)), you can use `levelrail-cli` for deeper validation:

```bash
levelrail-cli version   # compare running vs. latest published release
levelrail-cli status    # check Docker reachability and system config
```

Both commands talk to the control plane's API (`GET /api/v1/updates` and `GET /api/v1/system/status`), confirming it is reachable and authenticating correctly.

## Upgrading

**If you used install.sh:**

Re-run the same one-liner. It's safe to re-run: it overwrites the binary and unit file, then restarts the service.

```bash
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
```

To pin a specific release instead of the latest:

```bash
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.1.0 sh
```

**If you used Docker:**

Pull the new tag and recreate the container:

```bash
docker pull ghcr.io/glincker/levelrail:latest
docker compose up -d
```

Or if using `docker run`:

```bash
docker rm -f levelrail
# Re-run the docker run command from docs/docker.md
```

The named volume holding `/var/lib/levelrail-data` persists across recreation.

## Uninstalling

There is no uninstall script today.

**If you used install.sh:**

```bash
sudo systemctl stop levelrail
sudo systemctl disable levelrail
sudo rm /etc/systemd/system/levelrail.service
sudo systemctl daemon-reload

sudo rm "$(command -v levelrail)"        # or your LEVELRAIL_INSTALL_DIR path
sudo rm -rf /var/lib/levelrail-data      # or your LEVELRAIL_DATA_DIR path
```

::: warning
Removing `/var/lib/levelrail-data` deletes all app and deploy state.
:::

This does not touch Docker itself or any containers, images, or volumes Levelrail created for your deployed apps. Remove those separately if needed.

**If you used Docker:**

```bash
docker rm -f levelrail levelrail-agent
docker volume rm levelrail-data
```

## Getting help

- Ran into a specific error? Check [Troubleshooting](troubleshooting.md) first.
- Everything else (bugs, questions, feature requests): open an issue on [GitHub](https://github.com/glincker/levelrail/issues).

## See also

- [Docker installation](docker.md) - Running Levelrail as containers
- [Getting started](getting-started.md) - Deploy your first app after installation
- [Upgrading](installing.md#upgrading) - How to keep Levelrail up to date
