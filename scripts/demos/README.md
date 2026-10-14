# Demo recordings

Deterministic demo media for the README and `docs/demo.md`. The CLI scenes
use a scripted mock (`mock-levelrail`), so they need no server, Docker or
secrets and produce the same pixels every run.

## CLI scenes (GIF)

Needs `vhs`, `ttyd` and `ffmpeg` (`brew install vhs ttyd ffmpeg`).

```
scripts/demos/render.sh
```

Output lands in `docs/assets/demos/`: `install-deploy`, `deploy-rollback`,
`db-create`, `templates`.

- `lib.tape` holds the shared theme (brand tokens from `brand.yaml`) and
  hidden setup. Each scene `Source`s it.
- `bin/levelrail-cli` is a symlink to `mock-levelrail`; the tapes put `bin/`
  on PATH, so the typed commands are the real ones from `docs/cli-reference.md`.
- Anything the mock does not script exits 2 with `mock: unscripted`, so a
  tape cannot silently show a command that does not exist.
- To change output, edit `mock-levelrail`; pace is `DEMO_PACE` (seconds).

## Dashboard scenes (video, then GIF)

Needs a running, seeded control plane, Python 3 with `playwright`
(`playwright install chromium`) and `ffmpeg`. These reuse the same login
flow as `scripts/screenshots/login_state.py`, no new browser dependency.

```
LEVELRAIL_URL=http://localhost:8098 python3 scripts/demos/ui/record.py /tmp/lr-ui
scripts/demos/ui/to-gif.sh /tmp/lr-ui docs/assets/demos
```

The seeded state is the one `scripts/screenshots/capture.sh` builds (an app
named `marketing-site` with three deploys and metrics). That pipeline needs
a Docker daemon and tears everything down on exit, so start a control plane
the same way and keep it up while recording. Set `DEMO_APP` for a different
app. The button locators in `record.py` match on name (redeploy, roll back)
and should be re-checked against the UI after label changes.

## Regenerate everything

```
scripts/demos/render.sh && LEVELRAIL_URL=http://localhost:8098 python3 scripts/demos/ui/record.py /tmp/lr-ui && scripts/demos/ui/to-gif.sh /tmp/lr-ui docs/assets/demos
```
