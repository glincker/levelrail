# Docs OG cards

Per-page 1200x630 PNG cards for the docs. Needs Node and Google Chrome (`CHROME=/path` to override). ImageMagick `magick` is used if present to keep files small (`OG_QUANTIZE=0` skips it).

```
node scripts/og/generate.mjs                 # default card + top pages
node scripts/og/generate.mjs getting-started # one page
node scripts/og/generate.mjs --all           # every page in docs.json
node scripts/og/generate.mjs --wire          # also add og:image/twitter:image frontmatter
```

Output goes to `docs/public/og/<slug>.png` (slashes become dashes), served at `<docs_url>/public/og/<slug>.png`. Name, docs host and logo come from `brand.yaml`. `render.mjs` in this folder is the separate renderer for brand and social images.
