---
title: Install from mainland China
description: "Install Levelrail where github.com, ghcr.io or Docker Hub are slow or blocked, using a release base URL, Docker mirrors, a pinned version and signature checks."
"og:url": "https://docs.levelrail.com/install-from-china"
---

Outcome: Levelrail installs and runs on a server where github.com, ghcr.io or Docker Hub are slow or blocked, with checksum and signature checks still on.

<Note>
Before you start: the release tag you want (look it up from a machine that reaches GitHub), a host you control that serves the release files, and root access on the server. The general requirements are in [What you need](/what-you-need).
</Note>

github.com, ghcr.io and Docker Hub can be slow or unreachable from mainland China. This page lists the settings that let you install and run Levelrail through other hosts without giving up checksum and signature checks.

<Warning>
Speed from mainland China has not been tested by the project yet. Nothing here is a measured result. The mirror variables below are available from v0.9.4. Earlier releases do not read them.
</Warning>

## What Levelrail downloads

| What | Default host | How to change it |
| --- | --- | --- |
| Installer script | levelrail.com | Save the file once and copy it to the server |
| Release binary, `checksums.txt`, signature bundle | github.com | `LEVELRAIL_RELEASE_BASE_URL` |
| Docker Engine, when missing | get.docker.com | `LEVELRAIL_DOCKER_MIRROR` |
| Control plane and node agent images | ghcr.io | Docker daemon `registry-mirrors` or `LEVELRAIL_REGISTRY_MIRROR` |
| Build base images and BuildKit | Docker Hub | Docker daemon `registry-mirrors` or `registry.mirror_prefix` |

## 1. Pin the version

Look up the release tag on a machine that can reach GitHub, then pin it. A pinned install does not call the GitHub API to find the latest release.

```bash
export LEVELRAIL_VERSION=v0.9.4
```

## 2. Download release files from another host

`LEVELRAIL_RELEASE_BASE_URL` replaces `https://github.com/glincker/levelrail/releases/download`. The installer reads `<base>/<tag>/<asset>`, so your host must keep the same layout. Only `https` is accepted.

Copy `levelrail_*` binaries, `checksums.txt` and `checksums.txt.sigstore.json` byte for byte. The installer still checks the SHA-256 checksum and, when `cosign` is installed, the signature, whatever host served the files. A copy that was changed fails the check.

```bash
export LEVELRAIL_RELEASE_BASE_URL=https://downloads.example.com/levelrail
```

## 3. Install Docker through a mirror

When Docker Engine is missing the installer runs the official get.docker.com script. Set `LEVELRAIL_DOCKER_MIRROR` to `Aliyun` or `AzureChinaCloud` to pass that script's `--mirror` option.

```bash
export LEVELRAIL_DOCKER_MIRROR=Aliyun
```

## 4. Set Docker registry mirrors

Docker pulls images through the daemon, so set `registry-mirrors` in `/etc/docker/daemon.json` and restart Docker.

```json
{
  "registry-mirrors": ["https://mirror.example.com"]
}
```

```bash
sudo systemctl restart docker
```

`registry-mirrors` only applies to Docker Hub. It does not cover ghcr.io, so the control plane and agent images need step 5 or a pinned digest (step 6).

<Note>
DaoCloud runs a public mirror (`m.daocloud.io`, `docker.m.daocloud.io`, `ghcr.m.daocloud.io`). It is unofficial and community run, not operated or endorsed by Levelrail. It is best effort, can be slow or disappear, and sees which images you pull. Use any mirror you trust, and rely on the digest and signature checks in step 6, not on the mirror.
</Note>

## 5. Set a registry mirror prefix

`LEVELRAIL_REGISTRY_MIRROR` is read once by the installer and seeds the runtime setting `registry.mirror_prefix`. After that you change it live under Settings, in the CLI or through the API, with no restart. The server environment equivalent, used only as the initial value, is `APP_REGISTRY_MIRROR_PREFIX`.

The prefix is put in front of image references the control plane pulls for itself: the node agent image on new nodes and the BuildKit image.

```bash
export LEVELRAIL_REGISTRY_MIRROR=m.daocloud.io
```

Leave it empty to pull from the original registries.

## 6. Verify what you pulled

A mirror can serve the wrong bytes. Resolve the digest of the image from a trusted place, pull by digest, and verify the signature against the digest. The digest is the same on every host that copies the image faithfully.

```bash
cosign verify \
  --certificate-identity-regexp '^https://github.com/glincker/levelrail/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/glincker/levelrail@sha256:<digest>
```

Signature lookup needs to reach the registry that holds the signature. If ghcr.io is blocked, point the same command at a mirror that also copied the signature tags, or verify on a machine that can reach ghcr.io and copy the verified digest over. More commands are in [Verify a release](/verify-a-release#using-a-mirror).

## 7. Install and verify it works

Run the installer with the variables above exported, then check the service:

```bash
sudo --preserve-env=LEVELRAIL_VERSION,LEVELRAIL_RELEASE_BASE_URL,LEVELRAIL_DOCKER_MIRROR,LEVELRAIL_REGISTRY_MIRROR sh install.sh
curl -fsS http://127.0.0.1:8080/healthz
```

Expected: the installer ends with `Levelrail vX.Y.Z installed and running.` and `/healthz` answers. Save `install.sh` once from a machine that can reach it, as the table above says, and copy it to the server.

<Accordion title="If it does not work">

| What you see | What to do |
| --- | --- |
| `could not list releases from GitHub. Set LEVELRAIL_VERSION=vX.Y.Z to pin one.` | Export `LEVELRAIL_VERSION` as in step 1. |
| `checksum mismatch for ...` | The mirror changed a file. Copy the files again byte for byte. Do not skip the check. |
| Image pulls hang | `registry-mirrors` covers Docker Hub only. Use step 5 for ghcr.io images. |
| The mirror variables are ignored | They need v0.9.4 or later. |

</Accordion>

## Check from the dashboard

The doctor check "Release and image host reachability" tries github.com, ghcr.io and Docker Hub from the control plane with a short timeout. It warns and links here when one of them cannot be reached.

## Related

- [Installing](/installing)
- [Verify a release](/verify-a-release)
- [Runtime settings](/runtime-settings)
