---
title: "Add a template"
description: List your product or an open source app in the Levelrail template catalog with a pull request. Listing is free and the rules are the same for everyone.
"og:url": "https://docs.levelrail.com/add-a-template"
---

Anyone can add a template: a product vendor, a maintainer or a user of the app. You open a pull request on the public Levelrail repository that adds two files. Listing is free, nobody pays for placement, and the order in the directory is alphabetical. The rules below are the same for vendors and community contributors.

Target the open release branch (`release/v0.8.14` at the time of writing). A maintainer will retarget the pull request if you pick another branch.

## The two files

```text
internal/catalog/manifests/<id>/template.yaml
internal/catalog/manifests/<id>/compose.yaml
```

`<id>` is a lowercase slug (letters, digits, dashes) and is permanent once released. There is no logo step: the catalog uses a generic icon name from `template.yaml`, and the site draws the product mark from public icon sets. Do not add image files.

Do not add files under `internal/catalog/contrib/`. That older format is frozen.

## Worked example

`internal/catalog/manifests/acme-notes/template.yaml`:

```yaml
id: acme-notes
name: Acme Notes
description: A self-hosted notes app with sharing and full text search. Runs on SQLite in one container. On first open, the setup page asks you to create the admin login.
icon: book
category: Productivity
documentation_url: https://notes.acme.example/docs
source_repo: https://github.com/acme/notes
license: AGPL-3.0-only
license_class: network-copyleft
license_url: https://github.com/acme/notes/blob/main/LICENSE
hosted_use: unknown
listing:
  by: community
status: active
recommended_memory_bytes: 268435456
first_run:
  - Open the app and create the admin account on the setup page.
env:
  - key: TZ
    label: Time zone
    description: IANA time zone for dates and logs.
    default: Etc/UTC
domains:
  - service: notes
    port: 8080
    description: Public address of Acme Notes.
healthcheck:
  service: notes
  port: 8080
  path: /
```

`internal/catalog/manifests/acme-notes/compose.yaml`:

```yaml
services:
  notes:
    image: ghcr.io/acme/notes:1.4.2
    ports: ["8080"]
    environment:
      TZ: ${TZ}
      SECRET_KEY: $SERVICE_PASSWORD_64_NOTESKEY
    volumes:
      - notes_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 10
      start_period: 30s
```

Every `${KEY}` in the compose file needs a matching `env` entry, and every entry must be used. Secrets are generated at deploy time with `$SERVICE_PASSWORD_...` style variables, never written into the file. See [Compose apps](/compose-apps#templates) for the full manifest reference.

## Required fields

| Field | Rule |
| --- | --- |
| `source_repo` | The upstream source, as `https://github.com/<org>/<repo>`. |
| `license` | An SPDX id (for example `MIT`, `Apache-2.0`, `AGPL-3.0-only`) or `LicenseRef-<name>` for source-available and proprietary terms. |
| `license_class` | `permissive`, `copyleft`, `network-copyleft`, `source-available` or `proprietary`. It must agree with the id. |
| `listing.by` | `community`, or `vendor` (see below). |
| `status` | `active`. Maintainers set `deprecated` or `yanked`, with a reason. |
| `healthcheck` | The service, port and path that answers when the app works. Use `expect: 401` or `403` when the path sits behind a login, or `kind: tcp` for services with no HTTP surface. |

License is reviewed by maintainers. The catalog lists software that anyone may legally run from a public image, and labels it by class: open source (permissive, copyleft or AGPL), source available, or proprietary and free to self-host. It does not list images that need a registry login or a paid key to start, repackaged proprietary software, or software with no discoverable license.

## Eligibility rules

These come from the loader and the compose guards, and a pull request that breaks one fails CI.

- **Exact version tag.** `1.4.2`, not `latest`, `stable`, `edge`, `main`, `nightly`, `beta`, `rc` or a bare major such as `16`. A tag like `1.4` needs a maintainer exception.
- **Image source.** The image comes from the upstream project's own registry namespace (matching `source_repo`), a Docker Official Image, or an allowed rebuilder (LinuxServer.io). Anything else needs a maintainer exception and cannot be a vendor listing.
- **No digest in the file.** CI resolves the digest and records it in a generated lock file.
- **Healthcheck present**, and it must translate into a real probe.
- **No privileged containers, host network, host PID, added capabilities, device passthrough or container sockets** (the same compose guard every deploy uses).
- **Named volumes only.** No bind mounts, and each service keeps its own volumes.
- **No download or install at start.** An entrypoint or command that runs `curl`, `wget`, `git clone`, `pip install`, `npm install`, `apk add` or `apt-get` is rejected unless a maintainer grants an exception with a reason.
- **Published database ports need a generated credential**, or no host port at all.
- **No telemetry opt-out secrets or real credentials** in the files. Nothing secret goes in a template.
- **Arm64:** say in the pull request whether the image publishes `linux/arm64`. The catalog scan records it; you do not need to claim it in the file.
- **Maintainer contact:** put a way to reach you (GitHub handle) in the pull request.

## What CI checks

When a pull request changes files under `internal/catalog/manifests/`, the template workflow runs only for the changed template ids:

1. The loader and submission rules above.
2. The image tag resolves and lists the platforms it claims, and is under the size limit.
3. A vulnerability and secret scan of the image, joined with the CISA known-exploited list. A known-exploited hit or an unallowed secret blocks the pull request. A critical issue with a fix available is a warning.
4. A signature and attestation lookup, recorded when present and never required.
5. A smoke run: every service starts and stays up, and the declared health path answers.

Nothing runs on a schedule for your template.

## What Verified means

Every merged template is **Listed**. **Verified** means the project's own CI ran this exact image tag and digest on an amd64 host, all services stayed up, and the health path answered 200 to 399 (or the status you declared). The record is bound to the pinned tag, not to a date: when the tag changes the record stops matching until the new tag passes. **Verified and signed** adds a signature from your repository's build and a provenance attestation. Arm64 is reported separately as tested, image published or amd64 only.

Admission wording: no known-exploited vulnerability (CISA KEV) in this image when this version was accepted. It describes the moment of acceptance and links the CI run.

## Vendors

To show **Listed by the vendor**, set:

```yaml
listing:
  by: vendor
  vendor_org: acme
```

The badge means ownership and nothing else. A maintainer confirms it one of two ways: the pull request author is a public member of the GitHub organization that owns `source_repo`, or the domain of your `documentation_url` carries a DNS TXT record `levelrail-listing=<template id>`. Vendor pull requests meet the same review bar as any other, and vendors cannot buy a trust level. Proprietary software can only be listed by its vendor.

## Check it locally

```bash
scripts/template-lint.sh <id>            # validate the files and the submission rules
scripts/template-lint.sh <id> --smoke    # also start it under Docker and check it stays up
```

## Updates and takedowns

A version bump is a pull request that changes the tag in `compose.yaml`. Maintainers can mark an entry `deprecated` (still installable, with a warning) or `yanked` (removed from the install list), with a public reason. A vendor can ask for a yank of its own listing.
