# Docs OG cards

Per-page 1200x630 PNG cards for the docs. Needs Node and Google Chrome (`CHROME=/path` to override). ImageMagick `magick` is used if present to keep files small (`OG_QUANTIZE=0` skips it).

```
node scripts/og/generate.mjs                 # default card + top pages
node scripts/og/generate.mjs getting-started # one page
node scripts/og/generate.mjs --all           # every page in docs.json
node scripts/og/generate.mjs --wire          # also add og:image/twitter:image frontmatter
```

Output goes to `docs/public/og/<slug>.png` (slashes become dashes), served at `<docs_url>/public/og/<slug>.png`. Name, docs host and logo come from `brand.yaml`. `render.mjs` in this folder is the separate renderer for brand and social images.

## Social card renderer

`node scripts/og/render.mjs [name ...]` renders brand and social images (all of them with no names). Every card shares one look: Inter 300 headline with a gradient accent line, grain-masked glows, a floor gradient, mono pills and the dashboard in a tilted glass frame. `OUT_DIR=/some/dir` writes everything under that directory (same relative paths) so test renders touch nothing tracked.

- `og-home` / `og-home-light`: hero card, dark and light twin, written to `docs/assets/og-home.png` and `og-home-light.png` (light is for README `<picture>` use).
- `og-home-iso` / `og-home-iso-light`: alternate card with the isometric scene and install command; scene source is `scripts/og/assets/iso-hero.svg`.
- Other variants (`og-docs`, `og-install`, `github`, `square`, ...) keep their own copy and screenshot; see `COPY` and `VARIANTS` in `render.mjs`.
- `generate.mjs` uses the same look for the per-page docs cards and honors `OUT_DIR` too.

```
node scripts/og/render.mjs && node scripts/og/generate.mjs --all   # into the repo
OUT_DIR=/tmp/og node scripts/og/render.mjs                         # scratch copy
```
