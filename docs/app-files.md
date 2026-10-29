---
title: "App files"
description: Ship inline config files with a service so apps that need a file on disk at start can run without a custom image.
"og:url": "https://docs.levelrail.com/app-files"
---

Many self-hosted apps read a config file at start. A service can declare `files:` and the platform writes them into the container before it starts.

## app.yaml

```yaml
services:
  status:
    image: twinproduction/gatus:v5.12.1
    port: 8080
    files:
      - path: /config/config.yaml
        mode: "0444"
        content: |
          web:
            port: 8080
      - path: /etc/app/secrets.toml
        secret: true
        content: |
          token = "${API_TOKEN}"
```

| Field | Meaning |
| --- | --- |
| `path` | Absolute path in the container. No `..`, not under `/proc`, `/sys` or `/dev`, no duplicates. |
| `content` | Inline text. |
| `mode` | Octal permission bits, default `0644`. |
| `secret` | Store the content encrypted like an env secret. `${VAR}` placeholders are filled from the service's resolved env (including secrets and generated values) at container create. `$$` is a literal `$`. A missing variable fails the start. The content is never returned by the API or logged. |

A plain file is written exactly as given, so `${...}` inside it stays literal.

## Limits

Operators set these in the environment: `APP_SERVICE_FILE_MAX_BYTES` (one file, default 64 KiB), `APP_SERVICE_FILES_MAX_TOTAL_BYTES` (per service, default 256 KiB) and `APP_SERVICE_FILES_MAX` (count, default 32).

## How it works

The container is created, the files are copied in with the Docker archive API, then it starts. This needs no host path or volume, so it behaves the same on remote nodes (the agent does the copy) and on Podman. A node whose agent predates files refuses the deploy instead of starting the app without its config. The files live in the container layer, so a recreate rewrites them. Files are owned by root: a non-root app needs a mode it can read, such as `0444`. They are not read-only mounts, and the image's root filesystem must be writable.

Changing a file changes the service's applied config, so it shows as a pending change and is applied by a restart. An app.yaml deploy replaces the whole list.

## CLI and API

```
levelrail-cli apps files ls <app>
levelrail-cli apps files set <app> --path /etc/app.conf --from-file ./app.conf [--mode 0644] [--secret] [--apply]
levelrail-cli apps files rm <app> --path /etc/app.conf [--apply]
```

`GET`, `PUT` and `DELETE /api/v1/apps/{name}/files`. Secret files appear with `redacted: true` and no content. `?apply=restart` recreates the app now. Writes need the sensitive write ability.

## Templates

A catalog template declares a Compose top-level `configs:` entry with inline `content:`. See [Starter kit templates](/templates).
