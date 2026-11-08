# Following long operations from the CLI

`backups trigger`, `backups restore`, `databases clone` and `databases major-upgrade` return as soon as the server accepts the work. Add a flag to follow it to the end:

| Flag | Meaning |
| --- | --- |
| `--wait` | Block until the operation ends. Exit 0 on success, 1 on failure, 6 if it is still running at the timeout. |
| `--watch` | Same, and print a live progress line on stderr (one line per change when stderr is not a terminal). |
| `--watch-timeout DURATION` | Stop waiting after this long. The operation keeps running on the server. |

On `databases clone`, `--wait` already sets how long the fresh backup is awaited, so only `--watch` follows the clone itself.

Progress goes to stderr, so `--json` output on stdout stays clean. Defaults come from `LEVELRAIL_WATCH_TIMEOUT` (30m) and `LEVELRAIL_WATCH_POLL` (2s). Five failed polls in a row end the watch with exit 3.

```bash
levelrail-cli backups trigger main --wait && echo "backup done"
levelrail-cli databases major-upgrade main --version 17 --confirm main --watch
```
