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

Linux, `amd64` or `arm64`, with systemd as the init system. `install.sh` runs a preflight table (root, architecture, OS, systemd, curl, Docker 24 or newer, RAM, free disk, ports 80/443/8080) and stops on a failed check unless you pass `--force`. It is tested in CI on Ubuntu 24.04 and Debian 12; other distros should work but aren't tested. Docker is installed automatically via `get.docker.com` if it's missing. Windows and non-Linux nodes aren't supported, by design (see the root `CLAUDE.md`'s non-goals).

**RAM, CPU, and disk**

`install.sh` warns below 1 GB of RAM (`LEVELRAIL_MIN_RAM_MB`) and fails below 10 GB of free disk on the data directory's filesystem (`LEVELRAIL_MIN_DISK_GB`); the control plane itself checks no minimum. `levelrail-cli doctor` (`GET /api/v1/system/doctor`) checks Docker reachability, free disk space (warns below 1 GiB by default), and data-directory writability, not a memory or CPU floor, and no official minimum has been benchmarked or published either; that measurement is Phase 5 work per the [roadmap](roadmap.md).

As a practical starting point, not a hard requirement: 1 vCPU / 1 GB RAM / 10 GB disk is enough to boot Docker and the control plane on a small single-node instance. BuildKit builds and whatever apps you deploy need headroom of their own on top of that, so 2 vCPU / 2 GB RAM / 20 GB disk is more comfortable in practice. Scale up from there based on what you actually run.

**Ports**

- **Control-plane node:** `80/tcp` and `443/tcp`, inbound, reachable from the internet. Port 80 specifically is required for Let's Encrypt's HTTP-01 challenge during ACME issuance; both are what embedded Caddy binds for ingress, and `GET /api/v1/system/doctor` checks both are free to bind. See [Domains and ingress: firewall](domains-and-ingress.md#firewall-ports-80-and-443) if one is blocked.
- **Agent-only nodes** (additional servers added later via [Multi-node](multi-node.md)): none. The agent dials *out* to the control plane and the control plane never initiates a connection, so a managed node needs no inbound ports open at all.
- `install.sh` can configure `ufw` for you with `LEVELRAIL_CONFIGURE_UFW=1` (see the table below), or open `80/tcp` and `443/tcp` yourself via your cloud provider's firewall, `ufw`, or `iptables`.

## Option 1: install.sh (recommended)

```
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
```

This is the same script linked from the root [README](../README.md). It:

- Runs the preflight table and stops on a failed check (`--force` continues anyway)
- Installs Docker via `get.docker.com` if missing
- Downloads the newest release for `linux/amd64` or `linux/arm64` and verifies its checksum. While no stable release exists yet, it installs the newest pre-release; pick explicitly with `LEVELRAIL_CHANNEL=stable|beta`. A release that ships no binary for your architecture is skipped with a warning
- Writes a `levelrail.service` systemd unit and starts it, then waits for `GET /healthz`
- Checks that ports 80 and 443 answer on the server's public IP, and prints the `ufw`/`firewalld` commands to open them if not (some providers never route a server's own public IP back to itself, so treat a failure there as a hint, not proof)
- Prints every dashboard URL, the one-time **setup token** for creating the first admin, and a reminder to back up `<data dir>/master.key`

Requires `curl`, `systemd`, and root access.

### First sign-in

Open one of the printed `http://<ip>:8080/login?setup=<token>` links. The login page switches to "Set up the admin account" on its own and the token is pre-filled. Lost the summary? Print the token again on the server:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data levelrail setup-token
```

The dashboard shows a "connection is not encrypted" banner until you point a domain at the server (Domains page, primary domain plus ACME) and set an `https://` **dashboard URL**. After that, sign-in over plain HTTP is refused. To recover if the https URL breaks, add `APP_ALLOW_INSECURE_LOGIN=true` with `sudo systemctl edit levelrail` (`[Service]` then `Environment=APP_ALLOW_INSECURE_LOGIN=true`) and restart.

To skip the setup token and create the admin non-interactively, set `APP_ADMIN_USERNAME` and `APP_ADMIN_PASSWORD` in the unit (again via `systemctl edit levelrail`) before the first start.

::: details Optional environment variables

| Variable | Default | What it does |
| --- | --- | --- |
| `LEVELRAIL_VERSION` | newest release | Pin a specific release tag |
| `LEVELRAIL_CHANNEL` | stable, else newest pre-release | `stable` or `beta` |
| `LEVELRAIL_INSTALL_DIR` | `/usr/local/bin` | Where the `levelrail` binary is installed |
| `LEVELRAIL_DATA_DIR` | `/var/lib/levelrail-data` | Control plane data directory (SQLite database, master key, setup token, generated `brand.yaml`) |
| `LEVELRAIL_BINARY_FILE` | unset | Install a local binary instead of downloading one |
| `LEVELRAIL_BINARY_URL` | unset | Download the binary from this URL instead (no checksum verification) |
| `LEVELRAIL_PUBLIC_IP` | discovered | Public IP used for the reachability test and summary |
| `LEVELRAIL_SKIP_REACHABILITY` | unset | Set to `1` to skip the port 80/443 test |
| `LEVELRAIL_MIN_RAM_MB` / `LEVELRAIL_MIN_DISK_GB` / `LEVELRAIL_MIN_DOCKER_MAJOR` | `1024` / `10` / `24` | Preflight thresholds |
| `LEVELRAIL_HEALTH_WAIT` | `60` | Seconds to wait for the service to become healthy |
| `LEVELRAIL_CONFIGURE_UFW` | unset (off) | Set to `1` to allow SSH, then 80/443, then enable `ufw` if it wasn't already active. The script never touches your firewall otherwise. |

:::

Common scenarios:

::: code-group
```bash [Pin a specific release]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.2.0-beta.5 sh
```

```bash [Custom data directory]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_DATA_DIR=/data/levelrail sh
```

```bash [Configure UFW automatically]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_CONFIGURE_UFW=1 sh
```

```bash [Ignore a failed preflight check]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo sh -s -- --force
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

`install.sh` already fails loudly if the control plane doesn't come up healthy within 60 seconds (`LEVELRAIL_HEALTH_WAIT`). You can re-check the installation at any time:

**Quick checks (unauthenticated):**

```bash
# Health check the control plane responds
curl -fsS http://127.0.0.1:8080/healthz

# true until the first admin account exists
curl -fsS http://127.0.0.1:8080/api/v1/auth/setup-status

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

Run the `upgrade` subcommand. It replaces the binary with the newest release, keeps your unit file (and any `systemctl edit` overrides) and data, restarts the service, and waits for it to come back healthy.

```bash
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh -s upgrade
```

Re-running the installer without arguments also works: it repairs the installation and rewrites the unit file.

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

**If you used install.sh:**

```bash
# removes the service, unit file, and binary; keeps /var/lib/levelrail-data
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh -s uninstall

# also deletes the data directory (database, master key, certificates)
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh -s uninstall --purge
```

::: warning
`--purge` deletes all app and deploy state, and the master key every stored secret is encrypted with.
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
