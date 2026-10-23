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

`node scripts/og/render.mjs [name ...]` renders brand and social images. `OUT_DIR=/some/dir` writes every output under that directory (same relative paths) instead of the repo, so test renders touch nothing tracked (the favicon .ico step still uses the repo).

- `og-home-iso`: dark 1200x630 hero card with the isometric scene, install command and grainy glows (grain-masked radial gradients). Output `docs/assets/og/og-home-iso.png`.
- `og-home-iso-light`: light twin for README `<picture>` use. Output `docs/assets/og/og-home-iso-light.png`.
- `og-home-dash`, `og-home-dash-light`: same look with the original copy ("Your own Vercel.") and the populated dashboard in a tilted glass frame (`dashboard-home.png` / `dashboard-home-light.png`, offset in the frame to crop the attention banner). Outputs `docs/assets/og/og-home-dash.png` and `og-home-dash-light.png`.
- Scene source: `scripts/og/assets/iso-hero.svg`, a static copy of the site hero (resting state, token parked, no animation).
- Review samples: `docs/assets/og/preview/`.

```
OUT_DIR=/tmp/og node scripts/og/render.mjs og-home-iso og-home-iso-light
```
