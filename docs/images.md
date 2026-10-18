---
title: "Images"
description: "See every container image on a node with its size, layers and deploy lineage, promote an image between environments by digest without rebuilding, free disk space without losing a rollback target, and check image signatures."
---

Every deploy leaves an image behind. The **Images** page shows what is on a node, which apps and deploys each image came from, and what can be removed.

## Build once, promote by digest

Promoting an app from staging to production reuses the exact image staging runs. Nothing is rebuilt, so what you tested is what ships. An image pulled from a registry is pinned to its digest at deploy time, and a promotion carries that digest to the target app. The image's lineage lists the build, each promotion and each rollback.

## Where to find it

<Tabs>
<Tab title="Dashboard">

**Images** (Deploy section of the sidebar). The table lists each image with size, the apps that use it and its status. Pick a row to see:

- **Lineage**: every deploy that used the image, with app, source (git push, manual, promoted, rollback), branch, commit and time.
- **Layers**: size of each layer and its share of the image, largest first to find what to slim down.
- **Verify signature**: checks the image against your public key.

The **Free** button removes what is safe to remove, after showing you the list. Choose a node in the top right to see another server.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli images list [--node N] [--keep N]
levelrail-cli images layers <image-id> [--node N]
levelrail-cli images gc [--node N] [--keep N] [--min-age-hours H] [--apply]
levelrail-cli images verify (<image-id> | --ref repo@sha256:...) [--node N]
```

`images gc` only previews until you add `--apply`. Every command accepts `--json` and `-o`. `images verify` exits non-zero when the signature is missing or does not match.

</Tab>
<Tab title="API">

```text
GET  /api/v1/images?node=&keep_per_app=
GET  /api/v1/images/layers?id=&node=
POST /api/v1/images/gc       {"node": "", "keep_per_app": 3, "min_age_hours": 24, "dry_run": true}
POST /api/v1/images/verify   {"id": "sha256:...", "node": ""}
```

Listing and verifying need read access. Cleanup needs root.

</Tab>
</Tabs>

## What cleanup never removes

An image is kept when any of these is true:

- it is the live image of an app
- it is a pinned rollback target
- it is one of the newest successful deploys of any app (default 3 per app)
- a container uses it
- it is younger than the minimum age (default 24 hours)
- this platform did not build or deploy it (for example a database image you pulled by hand)

Everything else is listed as removable. Set the defaults with `APP_IMAGE_KEEP_PER_APP` and `APP_IMAGE_MIN_AGE_HOURS`.

## Signature checks

Images signed with a cosign key pair (ECDSA P-256) can be verified against your public key. Set `APP_IMAGE_VERIFY_PUBKEY_FILE` to the PEM file. Verification then works from the image page and the CLI.

Set `APP_IMAGE_VERIFY_MODE=enforce` to also require a valid signature on every promotion. An image that is unsigned, signed by another key or not pinned to a registry digest is refused. Images built on the server have no registry digest and cannot be promoted while enforcement is on.

## Limits

- Per node inventory works for the control plane's own node today. Remote agents answer once they report images.
- Signature checks read the `sha256-<digest>.sig` tag cosign pushes next to the image. Keyless (Fulcio) signatures are not supported.
