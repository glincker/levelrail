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
- The app container runs as root, so the new volume is writable without an ownership step.
- Only source builds mount it. Image deploys do not know the layout and are left alone.
- Switching back to `none` leaves the volume in place until the app is deleted.
- Deleting the app removes the volume. It is a cache, so no `delete_data` confirmation applies, and only the control plane's own node is cleaned.
