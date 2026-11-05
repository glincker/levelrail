---
title: "Verify a release"
description: "Check that a Levelrail binary or image came from this repository's release workflow: checksum, cosign signature, build provenance and SBOM, with exact commands."
"og:url": "https://docs.levelrail.com/verify-a-release"
---

Every release is built by one GitHub Actions workflow, `.github/workflows/release.yml`. This page shows how to check a download or an image against it. The installer and `levelrail self-upgrade` run the checksum and signature checks for you. Use the commands below when you want to check by hand, for example before you copy a binary onto an air-gapped host.

## What is signed

| Artifact | What the workflow does | How you check it |
| --- | --- | --- |
| `checksums.txt` | Lists the SHA-256 of every CLI, agent and control plane binary and the install scripts. Signed with cosign (keyless), bundle published as `checksums.txt.sigstore.json`. | `cosign verify-blob`, then `sha256sum --check` |
| Release binaries | A GitHub build provenance attestation is created for every file listed in `checksums.txt`. | `gh attestation verify` |
| Container images `levelrail` and `levelrail-agent` | The manifest list digest is signed with cosign (keyless). | `cosign verify` |
| Image SBOM and provenance | BuildKit attaches an SPDX SBOM and SLSA provenance (`mode=max`) to each image. | `docker buildx imagetools inspect` |

Signing is keyless. There is no long-lived key to rotate or lose. The signature proves the file was produced by a run of this repository's release workflow on GitHub.

Releases cut before signing was added carry no bundle.

## Verify a binary

You need `curl`, `cosign`, `sha256sum` and, for the last step, the GitHub CLI (`gh`).

```bash
V=v0.1.0   # the release tag
BASE=https://github.com/glincker/levelrail/releases/download/$V
curl -fsSLO $BASE/checksums.txt -O $BASE/checksums.txt.sigstore.json -O $BASE/levelrail-linux-amd64

cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/glincker/levelrail/\.github/workflows/release\.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

sha256sum --check --ignore-missing checksums.txt

gh attestation verify levelrail-linux-amd64 --repo glincker/levelrail
```

Read the steps in order:

1. `cosign verify-blob` proves `checksums.txt` was signed by the release workflow of this repository.
2. `sha256sum --check` proves your binary matches a line in that file.
3. `gh attestation verify` proves GitHub recorded a build provenance attestation for the binary from this repository.

Stop at the first step that fails and do not run the binary.

## Verify an image

```bash
cosign verify ghcr.io/glincker/levelrail:beta \
  --certificate-identity-regexp 'https://github.com/glincker/levelrail/\.github/workflows/release\.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Use `ghcr.io/glincker/levelrail-agent` for the node agent. Tags are listed in [Installing](/installing#option-2-docker). Pin an exact `:vX.Y.Z` tag for anything that must not move, because `:latest` and `:beta` change.

## Read the SBOM and provenance

SBOM (SPDX) and provenance (SLSA) are BuildKit attestations inside the image. `cosign download sbom` does not find them. Read them with:

```bash
docker buildx imagetools inspect ghcr.io/glincker/levelrail:beta --format '{{ json .SBOM }}'
docker buildx imagetools inspect ghcr.io/glincker/levelrail:beta --format '{{ json .Provenance }}'
```

## What the server checks for you

`install.sh` always checks the SHA-256 checksum. It checks the signature when `cosign` is installed and the release has a bundle. `levelrail self-upgrade` and the dashboard upgrade verify the signature inside the server and need no `cosign` binary. After the first verified signature the host is pinned and refuses unsigned releases. The settings and the escape hatches are in [Installing](/installing#verifying-release-binaries).

## What this does not cover

- A valid signature says where a file was built. It does not say the code is free of bugs or vulnerabilities.
- It does not cover your own apps or the images you deploy. Build provenance for those is a separate, weaker record: see [Supply chain](/supply-chain).
- The SBOM and provenance attached to images are not checked by a separate signature command on this page.
- The identity pattern accepts any ref of the release workflow in this repository. Pin an exact tag in the pattern if you need to accept one release only.
- No third-party review of the build pipeline is claimed.

## Related

- [Security overview](/security)
- [Supply chain](/supply-chain)
- [Threat model](/threat-model)
- [Security advisories](/security-advisories)
