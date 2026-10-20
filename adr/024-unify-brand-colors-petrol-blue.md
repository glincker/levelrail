# ADR 024: One brand accent (petrol-blue) across /web and /docs, superseding ADR 022

Status: Accepted

Date: 2026-10-02

## Context

ADR 022 (2026-09-30) deliberately kept two brand colors: petrol-blue as the
identity color (the mark itself) and amber ("Rail signal") as the public
docs/marketing site's interactive accent, reasoning that a full docs retheme
was expensive surgery for a cohesion gain a documented two-color system
achieved more cheaply, and that amber's rail-signal metaphor was worth
keeping on its own terms.

Two days later, a broader visual-language pass (gradient-stroke buttons,
glass surfaces, a new Lora/Plus Jakarta Sans type system, inspired by
mastra.ai and an existing GLINCKER sibling site's dark-glass aesthetic)
made the two-color split's cost/benefit different from what ADR 022 assumed:

- The retheme ADR 022 called "real surgery" turned out to be mechanical
  once the token layer existed: `custom.css`'s accent tokens (`--vp-c-brand-1/2/3`)
  already routed every link/button/badge/hover-state through one place,
  so repointing them at petrol-blue was a token swap, not a rewrite of
  every selector.
- The broader redesign was happening anyway (fonts, component mechanics,
  glass surfaces), so the marginal cost of also unifying color was near
  zero, whereas doing it later as a standalone change would have been the
  "real surgery" ADR 022 described.
- `primary_color` stopped being unused dead code in the same pass (see
  below), which was one of ADR 022's own supporting observations for why
  a two-color split was low-cost to maintain -- that observation no longer
  holds once the color is live.

## Decision

Petrol-blue (`#107292` light / `#2fb3dc` dark, both now in `brand.yaml` as
`primary_color`/`primary_color_dark`) is the one brand accent everywhere:
the mark, `/docs`'s interactive accent (replacing amber), and `/web`'s
accent going forward. Amber is fully retired as a brand color and now
exists only as semantic warning-state color (severity chips, expiring-cert
indicators), the same red/amber/green convention ADR 022 already
distinguished from brand usage.

**`primary_color` is now live, not dead.** `web/src/components/BrandProvider.tsx`
sets `--brand-accent`/`--brand-accent-dark` as CSS custom properties on
`document.documentElement` from the `/api/v1/brand` response at boot --
previously the field was fetched and typed but nothing ever read it. This
is deliberately scoped to those two new variables only, not wired into
`/web`'s existing shadcn `--primary`/`--accent` tokens: that is a separate,
larger migration touching every existing screen's primary buttons and
focus rings, left for its own future ADR exactly as ADR 022 anticipated
("if the dashboard ever adopts a brand accent of its own, that decision
supersedes this ADR's 'dashboard stays neutral' clause via its own ADR").
This ADR takes that step for the identity color becoming live and for
`/docs`; it does not yet retheme `/web`'s existing component library.

**`/docs` is a full retheme, not a token swap alone.** Beyond
`--vp-c-brand-*`, this includes: fonts (Lora for display/headline, Plus
Jakarta Sans for body/UI, replacing Inter/Hanken Grotesk), a gradient-stroke
button mechanism (two stacked `background-clip: padding-box, border-box`
layers, not a flat fill), glass surface treatment for the nav and search
modal (blur/saturate/shadow tuned per ADR's own testing, with light-mode
borders deliberately dark-tinted rather than the common white-tinted
mistake that reads as invisible on a light background), and the
`HeroField.vue` WebGL shader recolored from amber to petrol-blue.

**The hero stays on its own fixed dark palette, not the site's light/dark
toggle.** A light-mode sky palette was built and tried (a desaturated pale
day sky, then an evening/sunset variant) and both read as decorative
rather than appropriate for an infrastructure tool once seen rendered. The
hero's WebGL field, text colors, scrim, and badge now use dedicated
`--hero-*` tokens fixed to the dark treatment regardless of the page's own
`.dark` class, rather than continuing to tune a day palette. This is a
narrower, deliberate exception to "the site follows the user's light/dark
preference," scoped to the hero section only -- everything else on the
page (nav, feature grid, footer, doc pages) still fully respects the
toggle.

## Rejected alternatives

- **Keep ADR 022's two-color split, only update component mechanics
  (fonts/buttons/glass) without touching color.** Rejected: would have
  shipped a docs site with a new premium component language still painted
  amber, the exact "orange everywhere, blue only in the tiny logo" problem
  ADR 022 itself flagged as looking accidental, just with nicer buttons.
- **A light-mode-aware hero (sky palette that flips with the toggle).**
  Tried twice (pale day sky, then warm sunset) and rejected after visual
  review both times. The shader's `u_light` uniform and day-palette
  `mix()` calls are left in `HeroField.vue` rather than removed, in case
  this is revisited later; they are simply never selected (`u_light` is
  now always `0.0`).
- **Wire `--brand-accent` into `/web`'s existing shadcn tokens now, while
  already touching `BrandProvider`.** Rejected for this pass: ADR 022 already
  named this as a separate, real decision deserving its own sign-off, and
  nothing about this redesign's docs-focused scope changes that -- doing
  it opportunistically here would be exactly the "silent edit" ADR 022
  warned against for its own superseding clause.

## Consequences

- `brand.yaml` gains `primary_color_dark` (optional, falls back to
  `primary_color` when unset, both in the YAML file and via a new
  `APP_BRAND_PRIMARY_COLOR_DARK` env override) -- `internal/brand/brand.go`,
  covered by new cases in `internal/brand/brand_test.go`.
- `web/src/types/brand.ts` gains `PrimaryColorDark?: string`, optional for
  the same existing-test-fixture reason as `SupportEmail`.
- `.feature-dot--warn` and `.custom-block.warning`'s background in
  `docs/.vitepress/theme/custom.css` were caught using the old amber
  `--vp-c-brand-1` value for genuine semantic warning meaning, not brand
  decoration, during the retheme and fixed to an explicit amber hex
  instead of the (now blue) brand token -- a reminder that a brand-color
  swap must audit for selectors that borrowed the old color's hue for a
  non-brand reason.
- `docs/.vitepress/theme/tokens.css` is a new, framework-agnostic token
  file (plain CSS custom properties, no VitePress/Tailwind-specific
  syntax) holding the color/radius/blur/button-mechanism values, headed as
  one of two synced copies (the other: `web/src/styles/glinui-tokens.css`)
  pending an eventual shared `glinui` package neither app can import from
  yet (`glinr-frontend` and `thegdsks` are separate repos/workspaces).
- If `/web`'s existing component library is ever retheme to consume
  `--brand-accent` directly, that is its own future ADR, per the
  "no silent edit" rule this ADR itself just followed.