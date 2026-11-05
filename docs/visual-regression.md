# Visual regression run

`scripts/visual/run.sh` opens the dashboard at 1280, 768 and 390 px for the routes in
`scripts/visual/routes.json`, saves a full page screenshot of each, and checks page
geometry. It exits 1 when an assertion fails and prints a markdown table.

## Run it

```sh
scripts/visual/run.sh                       # all routes, all widths
scripts/visual/run.sh --only dashboard,apps --widths 390
```

Needs Go, Node with `web/node_modules`, Python 3 with `playwright`, and Google Chrome
(falls back to Playwright's bundled Chromium). Nothing is added to `web/package.json`.

The script builds the control plane to a temp dir, starts it in dev mode (user `dev`,
password `dev`, fixture tokens) on a free port with a throwaway data directory, starts
Vite against it on another free port, seeds two apps through the API, runs the
capture, then stops both and deletes the data. It never touches a running instance.
`VISUAL_BIN` reuses a prebuilt binary, `VISUAL_OUT` moves the output.

Output goes to `web/visual-out/` (gitignored): `<route>-<width>.png` and `report.md`.

## What it checks

Failures (exit 1):

- the page scrolls horizontally
- an element's right edge is past the viewport and no ancestor scrolls or clips it
- an absolutely positioned node with no insets overlaps a flow sibling, or two such
  nodes share one spot (the shape of the marketing site diagram regression)
- redirected to login, the Status advisory block missing, the sidebar without group
  labels, the command palette not opening

Warnings (need a human look): an element leaving its non-clipping parent, clipped text,
broken images, console errors, a page that never goes network idle.

Animations are disabled and `prefers-reduced-motion` is set before capture. Dark scheme
only for now.

## Not done yet

- Baseline images and pixel diffing (`LEVELRAIL_VISUAL_MAX_DIFF_PCT`). Baseline storage in
  the repo is an owner decision, so the run produces screenshots and geometry only.
- CI wiring: an opt-in job on pull requests touching `web/src/**/*.css`,
  `web/src/components/banners/**` or `web/src/components/shell/**`, as a new workflow
  through a branch and pull request. It needs Chrome on the runner and the Go build.
- A light scheme pass and masked dynamic regions for diffing.
