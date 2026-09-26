---
description: A software bill of materials for each Dockerfile build, an optional vulnerability scan in a short-lived container and a gate that can keep a release from going live. What it costs, what it does not cover and every APP_BUILD_ATTEST, APP_SCAN and APP_SBOM setting.
---

# Supply chain visibility

A Dockerfile build can record what is inside the image, and the control plane can check that list for known vulnerabilities before the release goes live. Both are off by default.

| Part | What it does | Default |
| --- | --- | --- |
| SBOM and provenance | BuildKit attaches a software bill of materials (SPDX or CycloneDX) and a minimal SLSA provenance record to the build. The control plane keeps a compact package list per deploy and the SBOM document on disk. | Off. Turn on with `APP_BUILD_ATTEST=true`. |
| Vulnerability scan | A one-shot Trivy or Grype container scans the SBOM. You get counts by severity and the most severe fixable findings per deploy. | Off per app. |
| Scan gate | What a scan may do to a release: `off` (report only), `warn` or `block_on_critical`. | `off`. |

Nothing runs while idle. There is no resident scanner: a scan starts a container, reads one file, prints JSON and the container is removed again.

## Turn it on

1. Set `APP_BUILD_ATTEST=true` on the control plane and restart it. From then on each Dockerfile build records an SBOM.
2. Open the app, then **Deploy settings**, then **Supply chain**, and turn on scanning. Or use the CLI:

```bash
levelrail apps scan enable web
levelrail apps scan gate web block_on_critical
levelrail apps scan status web
levelrail apps sbom web            # newest deploy that has an SBOM
levelrail apps sbom web da_abc123 --download --file web.sbom.json
levelrail apps scan run web da_abc123
levelrail apps scan disable web
```

Each deploy's **Supply chain** section (in the deployments drawer and on the per-app deploy page) shows the package count, the first packages, a license summary and vulnerability counts by severity, with a **Scan now** button and a download link for the SBOM.

## What a scan costs

- **Scanner image.** The first scan pulls the scanner image (default `docker.io/aquasec/trivy:0.65.0`, or `docker.io/anchore/grype:v0.99.1` with `APP_SCANNER=grype`). These images are roughly 100 to 250 MB. The size was not measured for this release, so treat it as a rough range. The image stays on the host after the scan.
- **Vulnerability database.** The scanner downloads its database on the first scan and refreshes it later. It is cached in a Docker volume named after the brand short name with a `-scanner-cache` suffix.
- **Per scan.** One container with `APP_SCAN_MEMORY_MB` (default 1024) of memory, `APP_SCAN_CPUS` (default 1) CPUs and a 256 process limit, all capabilities dropped, `no-new-privileges`, removed afterwards. It has network access, because the database download needs it. Only the SBOM is copied into it: no app data, secrets or Docker socket.
- **Deploy latency.** A deploy waits for the scan before it goes live, up to `APP_SCAN_TIMEOUT` (default 5 minutes). A scanner that fails or times out never blocks a release.
- **Build time.** Attestations add work to every build (BuildKit runs a scanner image over the build result). This was not measured on the fixture app for this release, which is why `APP_BUILD_ATTEST` is off by default.

## The gate

| Mode | Critical vulnerabilities found | Release |
| --- | --- | --- |
| `off` | recorded | goes live |
| `warn` | recorded, deploy flagged with the reason | goes live |
| `block_on_critical` | recorded | **does not go live** |

A blocked deploy fails with the reason, `desired state` is not changed and the previous release keeps serving. If the scanner is unavailable, times out or fails, the gate fails open and the deploy records that the scan did not complete.

To let one blocked release through, an operator arms an override with a reason (**Deploy settings**, **Supply chain**, or `levelrail apps scan override web --reason "..."`). It applies to the next release only, expires after `APP_SCAN_OVERRIDE_TTL` (default 1 hour), and the reason is recorded in the app timeline and the audit log. A deploy freeze still applies as usual: the gate runs after a build, so it never releases a frozen deploy.

## What is not covered

- Only Dockerfile builds get an SBOM. Railpack builds, image deploys, static sites and builds dispatched to a remote node record none. Those deploys show no supply chain section data.
- Provenance is `mode=min` and is recorded, not signed or verified.
- Scans run against the SBOM, so they find what the SBOM lists. They do not read the image.
- The scanner image references and versions above were not pulled or run against a registry while building this feature.

## Retention

SBOM documents live in `DataDir/supplychain/<app>/<deploy>.sbom.json`, never in the database. The control plane keeps the newest `APP_SBOM_KEEP_PER_APP` (default 20) documents per app. An older document is deleted but its record, with the package count, licenses and vulnerability counts, stays. Records and files are removed when the app is deleted or when the deploy they belong to no longer exists. An hourly sweep also removes orphan files and any scanner container older than twice the scan timeout.

## API

| Route | Purpose |
| --- | --- |
| `GET /api/v1/apps/{name}/supply-chain` | Scan settings and server state. |
| `PUT /api/v1/apps/{name}/supply-chain` | Change `scan_enabled` and `scan_gate`. |
| `POST /api/v1/apps/{name}/supply-chain/override` | Arm the one-shot override with a `reason`. |
| `GET /api/v1/apps/{name}/deployments/{id}/sbom` | SBOM summary. Add `?download=true` for the raw document. |
| `GET /api/v1/apps/{name}/deployments/{id}/vulnerabilities` | Scan result and gate decision. |
| `POST /api/v1/apps/{name}/deployments/{id}/scan` | Scan now. |

`GET /api/v1/apps/{name}/deploy-attempts` and `GET /api/v1/deployments` gain two nullable fields: `sbom_packages` and `vuln_counts`. The MCP server exposes `get_deploy_sbom` and `get_deploy_vulnerabilities`, both read only.

## Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BUILD_ATTEST` | `false` | Ask BuildKit for an SBOM and minimal provenance on Dockerfile builds. |
| `APP_SCAN_ENABLED` | `true` | Server kill switch. `false` refuses every scan and never starts a scanner container. |
| `APP_SCANNER` | `trivy` | `trivy` or `grype`. |
| `APP_SCANNER_IMAGE` | scanner default | Scanner image reference. |
| `APP_SCAN_TIMEOUT` | `5m` | Longest one scan may run. |
| `APP_SCAN_PULL_TIMEOUT` | `10m` | Longest the scanner image pull may take. |
| `APP_SCAN_MEMORY_MB` | `1024` | Memory cap of a scan container. |
| `APP_SCAN_CPUS` | `1` | CPU cap of a scan container. |
| `APP_SCAN_OVERRIDE_TTL` | `1h` | How long an armed override stays valid. |
| `APP_SBOM_KEEP_PER_APP` | `20` | SBOM documents kept per app. |
| `APP_SCAN_SWEEP_INTERVAL` | `1h` | How often the cleanup sweep runs. |
