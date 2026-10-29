---
title: "kubectl style CLI verbs"
description: Generic get, describe, delete, edit, port-forward and cp verbs, and how each maps onto the existing commands.
"og:url": "https://docs.levelrail.com/cli-kubectl-parity"
---

# kubectl style CLI verbs

If you already know kubectl, the same muscle memory works here. The generic verbs work on three kinds: `app`, `database` (alias `db`) and `node`. Plural forms are accepted.

## Verb mapping

| You type | What it does | Same as |
| --- | --- | --- |
| `get apps` | List apps from the uniform v2 view | `apps list` |
| `get app web -o yaml` | One app as YAML | `apps get web` |
| `get apps -l env=prod` | Label selector (`k=v`, `k!=v`, `k`, `!k`) | none |
| `get apps -o wide` | Adds image, port, domains, labels and version columns | none |
| `get apps -o name` | Prints `app/web` per line | none |
| `describe app web` | Spec, conditions and the 10 latest timeline events | `apps get`, `apps timeline` |
| `delete app web` | Deletes the app, keeps data volumes | `apps delete web` |
| `delete db main` | Deletes the database | `databases delete main` |
| `edit app web` | Opens the app document in `$VISUAL` or `$EDITOR`, then applies it | `export --app web`, `apply -f` |
| `apply -f app.yaml` | Plans and applies resource documents | existing |
| `port-forward web 8080:3000` | Local port to an app container port | `apps tunnel web --local-port 8080 --port 3000` |
| `port-forward db/main 5433` | Local port to a managed database | `databases tunnel main --port 5433` |
| `cp web/data:conf/app.ini ./app.ini` | Download one file from an app volume | `apps volumes cp` |
| `cp ./app.ini web/data:conf/app.ini` | Upload one file | `apps volumes cp` |
| `rollout status\|history\|undo\|restart` | Deploy lifecycle | existing |
| `api-resources`, `explain` | Document kinds and their fields | existing |

Every verb takes `--json`, `--output` and `--query` like the rest of the CLI. `-o` and `-l` are short forms of `--output` and `--selector`.

## Differences from kubectl

- `get -o wide` has real extra columns. `table` stays compact.
- There is no field selector: the API supports label selectors only.
- `delete` asks for confirmation on a terminal and needs `--yes` in scripts. It never destroys data volumes; use `apps delete --delete-data --confirm NAME` for that.
- `edit` supports apps. It compares the resource version before and after the editor session. If the app changed meanwhile, nothing is applied and your edit is saved to a temp file whose path is printed.
- `cp` copies single files, not directories, and the remote side is written `APP/VOLUME:PATH`.
- `port-forward` listens on loopback only.
- Nodes are read only through `get` and `describe`. Remove one with `nodes delete`.
