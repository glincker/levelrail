---
title: "Drop deploy for static sites"
sidebarTitle: "Drop deploy"
description: Drag a folder or a .zip onto a static site and it goes live, with no git and no build.
"og:url": "https://docs.levelrail.com/drop-deploy"
---

## What this does

Drop deploy publishes a folder of built files (or a `.zip` of it) as a static site. There is no git remote and no build step: what you drop is what is served. It uses the same publish path as a git-based static site, so `_redirects`, `_headers`, the single-page-app fallback and a custom `404.html` all work as described in [Static site routing](/static-routing), every drop appears in deploy history as an upload, and you can roll back to an earlier version.

## From the dashboard

- **New site:** open New app and use "Drop a static site". Drag a folder or a `.zip` in, or use Choose folder / Choose .zip. The site name is taken from the folder or file name (`My Blog.zip` becomes `my-blog`) and you can change it before publishing.
- **Existing static site:** the app's Overview page has a "Publish by dropping a folder" card. Dropping there replaces the live version with the new one.

Before anything is sent you see the file count and total size. If the folder has no `index.html` at its top, you get a warning, because the site root would show a 404. Errors say what to do: a site over the limits asks you to trim it or ask an admin to raise them, a name that is taken asks you to pick another, and an interrupted upload offers Try again. A site created by a drop that fails is removed again, so a failed attempt leaves nothing behind.

## From the CLI

```bash
levelrail-cli deploy ./site --static
levelrail-cli deploy site.zip
```

A `.zip` argument implies `--static`. The first run creates the site (named after the folder or file, override with `--app NAME`); later runs publish a new version. `--dry-run` lists what would be sent. A folder is packed the same way as a container deploy: `.gitignore`d files, `node_modules`, `.git` and `.env` files are skipped unless `--include` names them.

## What is accepted

- A zip is validated before anything is stored. Entries that try to leave the site (`../`, absolute paths, backslashes), symbolic links, devices and duplicate names are rejected.
- `__MACOSX/`, `.DS_Store` and `Thumbs.db` entries are ignored.
- If the whole site sits inside one top-level folder (the usual result of zipping a folder on macOS or Windows), that wrapper is removed so `index.html` ends up at the site root. This repeats for nested wrappers. If no level has an `index.html`, the layout is kept as is.
- Zips made by the dashboard from a dropped folder use the same rules, so `.git` and `node_modules` inside a dropped folder are left out.

## Limits

Limits come from the same environment variables as other source uploads and apply to zips too:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_UPLOAD_MAX_BYTES` | 512 MiB | Largest upload (the zip itself) |
| `APP_UPLOAD_MAX_FILES` | 50000 | Most files in one site |
| `APP_UPLOAD_MAX_EXTRACTED_BYTES` | 2 GiB | Largest site once unpacked, also the cap for a single file |

## Locked apps and rollback

A drop deploy goes through the normal deploy pipeline, so it follows the same rules as any other deploy: a freeze window or an approval still applies, and when an app is locked (see [Deploy safety](/deploy-safety)) the new version is built but held until you publish it. Earlier versions stay available for rollback.

## What it does not do

It does not run a build. If your project needs `npm run build`, build it first and drop the output folder (for example `dist`), or connect the repository and let Levelrail build it. It also does not keep a folder in sync: each drop is one complete version.
