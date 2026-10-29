---
title: "Volume files"
description: List, download, upload and delete single files in an app's named volume, and restore one file from a backup, without SSH.
"og:url": "https://docs.levelrail.com/volume-files"
---

Fix a config file or pull a log out of an app's named volume without logging in to the server. Every operation is confined to the volume: paths with `..`, absolute paths and symlinks that point outside the volume are refused.

## Dashboard

Open the app, then the Volumes tab. The Files card lists the volume, lets you open folders, download a file, upload a file into the current folder and delete a file (the delete button asks for a second click).

## CLI

```bash
levelrail-cli apps volumes ls web data conf
levelrail-cli apps volumes cp web data :conf/app.ini ./app.ini     # download
levelrail-cli apps volumes cp web data ./app.ini :conf/app.ini     # upload, replaces the file
levelrail-cli apps volumes cp web data --from-backup bkh_123 :conf/app.ini
levelrail-cli apps volumes rm web data conf/old.ini
```

The remote side starts with `:`. Use `-` for stdout or stdin. All commands accept `--json`.

## API

| Method and path | Ability |
| --- | --- |
| `GET /api/v1/apps/{name}/volumes/{volume}/files?path=&limit=` | read:sensitive |
| `GET /api/v1/apps/{name}/volumes/{volume}/files/content?path=` | read:sensitive |
| `PUT /api/v1/apps/{name}/volumes/{volume}/files/content?path=` (raw body) | write |
| `DELETE /api/v1/apps/{name}/volumes/{volume}/files/content?path=` | write |
| `POST /api/v1/apps/{name}/volumes/{volume}/files/restore` with `{"backup_id","path"}` | write |

Downloads, uploads, deletes and restores write an audit entry that includes the file path. Delete removes one file, never a directory.

## Behavior

- An upload goes to a temp file and is renamed into place, so a failed or oversized upload never leaves a partial file. Existing mode and owner are kept so the app can still read what it owns.
- Restoring a file reads the succeeded backup archive and writes only that one entry back. Other files are untouched.
- The app keeps running. Editing a file the app has open is your call.
- Remote nodes need an agent that includes this operation; an older agent answers with an error.

## Limits

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_VOLUME_FILE_MAX_UPLOAD_MB` | `64` | Largest file accepted by upload and restore. Enforced on the control plane and again on the agent. |
| `APP_VOLUME_FILE_LIST_LIMIT` | `1000` | Most entries returned for one directory. The response says when the list was truncated. |
