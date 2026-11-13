---
title: "Next.js image cache volume"
description: "Keep optimised Next.js images across deploys and replicas with an opt-in cache volume."
---

Next.js writes optimised images to `.next/cache/images` inside the container. Without a volume every deploy and every new replica starts cold and re-optimises each image. Opt in and Levelrail mounts a per app volume at that path.

## Enable

In `app.yaml`:

```yaml
services:
  web:
    next:
      image_cache: persistent   # default: none
```

Or on a running app, then redeploy from source:

```
levelrail-cli apps next-image-cache set web persistent
```

The saved value wins over `app.yaml`. Set `none` to stop mounting it on the next deploy.

## Behaviour and limits

- The volume is `app-<name>-next-images`, one per app. Replicas on the same node share it. Replicas on other nodes each keep their own volume.
- The mount path follows where the server writes. Plain `next start` builds mount `/app/.next/cache/images`. Apps with `output: 'standalone'` (no custom `distDir` or tracing root) run `.next/standalone/server.js`, which changes into its own folder, so the mount is `/app/.next/standalone/.next/cache/images`. A custom start command that changes the working directory, or a custom `distDir`, moves the cache and the mount will miss it.
- The volume is owned by the user the app runs as. Many Next.js Dockerfiles end with a `USER` line (for example `USER nextjs` or `USER 1001`), and a new volume starts owned by root. Before the first start Levelrail reads that user from the image (a name is looked up in the image's `/etc/passwd`, no `USER` means root) and gives it the volume root. See [Troubleshooting](#troubleshooting) when that is not enough.
- Only source builds mount it. Image deploys do not know the layout and are left alone.
- Switching back to `none` leaves the volume in place until the app is deleted.
- Deleting the app removes the volume. It is a cache, so no `delete_data` confirmation applies, and only the control plane's own node is cleaned.

## Troubleshooting

After each container start Levelrail checks once that the app's user can write the root of every volume it mounts. The result is the app's `VolumesWritable` condition, shown in the dashboard status, `levelrail-cli apps status` and the API, and as an `ensure_volume` step in the deploy trace.

`VolumeNotWritable` with a message such as `volume /app/.next/standalone/.next/cache/images is not writable by user nextjs (uid 1001)` means Next.js will log `EACCES` errors and optimise every image again on each request. Levelrail only hands a volume root to the app user when the root is still owned by root, so this shows up when:

- the volume was created by an older image that ran as another non-root user, so its root belongs to that uid
- the ownership step could not run on the node, for example because its helper image could not be pulled

Fix it in one of two ways:

- Create the directory in the Dockerfile before `USER`, owned by the runtime user, so a new volume starts with that owner: `RUN mkdir -p .next/cache && chown 1001:1001 .next/cache` (use the path from the message). Then set `image_cache` to `none`, delete the old `app-<name>-next-images` volume, set it back to `persistent` and redeploy. It is a cache, so nothing is lost.
- Or keep the volume and chown its root to the uid and gid in the message.

`Unverified` means the node's runtime could not run the check (today an app on a node other than the control plane's, or a user name the image's `/etc/passwd` does not list). It does not mark the app unhealthy. `APP_VOLUME_WRITE_PROBE=off` turns the check off.
