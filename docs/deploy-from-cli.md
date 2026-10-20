---
title: "Deploy from your computer"
sidebarTitle: "Deploy from CLI"
description: Deploy the current directory with one command, no git remote and no registry needed.
"og:url": "https://docs.levelrail.com/deploy-from-cli"
---

## What this does

`levelrail-cli deploy` packs a directory on your machine, uploads it to the control plane, builds it there and follows the deploy until the app is healthy. You need no git remote, no registry and no SSH access. It is the quickest way to try an app that is not in a repository yet.

The directory needs a `Dockerfile` or a stack Levelrail detects (Node, Go, Python, Java and others, built with Railpack). Docker Compose projects are not supported by this command yet. For a static site, see [Drop deploy](/drop-deploy): `levelrail-cli deploy ./site --static` or a `.zip`.

## Deploy a directory

Sign in once (`levelrail-cli login --device`, or set `APP_API_URL` and `APP_API_TOKEN`), then run this in the project folder:

```bash
levelrail-cli deploy
```

The first run creates an app named after the directory, with the port taken from the Dockerfile `EXPOSE` line or the detected framework. Running it again uploads the new contents and deploys them to the same app. Pass a path to deploy another folder, `--app NAME` to pick the app name, and `--port N` to override the port.

Before anything is sent you see what will be uploaded:

```text
deploy: /home/you/demo-app as app "demo-app" (Dockerfile)
  4 files, 107 B to upload (.gitignore, node_modules, .git and .env files are skipped)
  largest files:
          83 B  Dockerfile
          15 B  src/server.js
           6 B  .gitignore
           3 B  package.json
Proceed? [Y/n]
```

After you confirm, the CLI uploads the archive, streams the build log, waits for the deploy and prints the app URL on stdout:

```text
created app "demo-app" on port 3000
  uploading 100% (208 B of 208 B)
  uploaded 208 B as 3f9c1a0b52de
building "demo-app" (deploy dep_1)
step 1/2 FROM node:22-alpine
...
deploy dep_1: healthy
https://demo-app.203-0-113-10.sslip.io
```

The exit code is 0 for a healthy deploy, 7 when the deploy failed or rolled back and a timeout code when `--timeout` (default 15 minutes) ends before the deploy finishes. The deploy itself keeps running after a timeout.

## What is uploaded

Everything in the directory except:

- files matched by `.gitignore` (the root one and nested ones),
- `.git`, `node_modules`, `.next`, `.nuxt`, `.turbo`, `.venv`, `venv`, `__pycache__`, `target`, `coverage`, `.terraform` and `.DS_Store`,
- secret-looking files: `.env`, `.env.*` (except `.env.example`, `.env.sample` and `.env.template`), `*.pem`, `*.key`, `id_rsa*` and `id_ed25519*`.

A `.gitignore` rule that starts with `!` cannot bring a secret file back. Symbolic links and special files are never uploaded. To send an ignored path on purpose, name it with `--include`, for example `--include .env`.

`--dry-run` lists the files that would be uploaded and changes nothing:

```bash
levelrail-cli deploy --dry-run
```

## Environment and build arguments

```bash
levelrail-cli deploy --env LOG_LEVEL=info --build-env NODE_ENV=production
```

`--env` sets runtime variables on the app and is repeatable. `--build-env` passes Dockerfile `ARG` values to the build and is repeatable. For secrets use `levelrail-cli apps secrets set`, not `--env`, because env values are visible in the app's env list.

## Size limits and retention

| Setting | Default | Where |
| --- | --- | --- |
| `APP_UPLOAD_MAX_BYTES` | 512 MiB (compressed) | CLI and control plane |
| `APP_UPLOAD_MAX_FILES` | 50000 files | control plane |
| `APP_UPLOAD_MAX_EXTRACTED_BYTES` | 2 GiB after extraction | control plane |
| `APP_UPLOAD_RETENTION` | 168h | control plane |

When the directory is over the cap the CLI stops before uploading and says which variable to raise. Excluding build output in `.gitignore` is usually the better fix.

The control plane keeps each uploaded archive under its data directory for the retention period, then deletes it. Uploading identical content again reuses the stored archive. Deploy history shows each upload as "Uploaded" with the archive hash and file count, and no git commit.

## Roll back or redeploy an earlier upload

Every build is tagged with its archive hash, so the normal rollback works:

```bash
levelrail-cli rollback demo-app
```

To rebuild an earlier upload from the stored archive without sending the directory again, pass its hash from the app overview or deploy history:

```bash
levelrail-cli deploy --app demo-app --upload-hash 3f9c1a0b52de...
```

This only works while the archive is still within the retention period; after that, deploy the directory again.

## Security

- The upload endpoint needs the `deploy` ability on the token or session, and every upload is written to the audit log. Archive contents are never logged.
- Archives are checked before they are stored and again when extracted: absolute paths, `..` segments, symbolic links, hard links, devices and other special files are rejected, and file count and expanded size are capped so a small archive cannot expand into a disk-filling bomb.
- Stored archives live in the control plane data directory, never in a web-served path.

## Related

- [Deploying apps](/deploying-apps) covers git sources, images, rollbacks and promotion.
- [Getting started](/getting-started) installs the control plane and the CLI.
