---
title: GLINR design tokens
description: The single source of truth for color, space, radius, elevation, type and motion in the Levelrail dashboard, and how to add or change a token.
---

# GLINR design tokens

The GLINR design tokens are the single source of truth for color, space, radius, elevation, type, motion and content widths in the dashboard. The source is `web/src/styles/tokens/glinr.tokens.json`; everything else is generated from it.

The source is intentionally framework-neutral (a DTCG-like JSON shape, no CSS or Tailwind specifics), so other GLINCKER products can reuse the same file and generate their own output.

## Page structure

Rules every dashboard page follows. Tokens say what a value is; these say where things go.

1. The `PageTemplate` hero is the only title and description on a page. The first card never repeats either. Provider connection cards render no header of their own under the hero.
2. Cards are single level. No bordered boxes inside a `Card`, except a semantic callout, which must be `ui/banner.tsx`. Lists inside a card use `divide-y`, not a box per row.
3. Vertical rhythm inside a card is `space-y-4` between blocks and 24px (`space-y-6`) between sections.
4. Action rows sit 16px below the content they act on (`border-t pt-4`, see `ConnectionActions`) and are aligned the same way on every page, primary action last.
5. Empty states are compact: `EmptyState compact` (icon, one line, one muted line, no dashed outline, about 96px) inside a page section. The full dashed state is for a whole empty page.
6. A callout never contains the primary form field. The banner explains; the labelled field and its Save button sit below it as their own block.

Help links live in the hero `actions` slot, not beside the title badge. A `Banner` with only a title renders it as 14px regular body text; 14px 600 is for banners that also have a description.

## Files

| File                                      | Role                                                              |
| ----------------------------------------- | ----------------------------------------------------------------- |
| `web/src/styles/tokens/glinr.tokens.json` | Source. Edit this.                                                |
| `web/src/styles/glinr-tokens.css`         | Generated `--glinr-*` variables, `:root` plus `.dark`.            |
| `web/src/styles/glinr-theme.css`          | Generated Tailwind v4 `@theme inline` mapping.                    |
| `web/src/styles/glinr-fonts.css`          | Hand written `@font-face` for Geist and Geist Mono (latin, swap). |
| `web/scripts/build-tokens.mjs`            | Generator. `npm run tokens` in `web/`; also runs as `prebuild`.   |

A vitest test (`web/scripts/build-tokens.test.mjs`) fails if the committed CSS drifts from the source, if a legacy variable disappears, or if any `var(--glinr-*)` in `web/src` names a variable that does not exist.

## Naming rules

- Variables are `--glinr-<group>-<name>`, for example `--glinr-color-fg`, `--glinr-space-4`, `--glinr-motion-fast`.
- Variables that predate the groups keep their original names through `$extensions.glinr.name` (`--glinr-bg`, `--glinr-radius`, `--glinr-elev-card`). Never rename or remove one; add a new token and migrate callers instead.
- Each token has a `$description` and a `$extensions.glinr.stability` of `stable` or `experimental`. Experimental tokens may change value or be removed after a release note.
- A token value is plain, or `{ "light": ..., "dark": ... }`, or an alias such as `{space.6}`.
- `$extensions.glinr.tailwind` emits a Tailwind v4 theme variable with a `glinr` namespace (`text-glinr-sm`, `bg-glinr-surface`, `max-w-glinr-content`) so it cannot collide with Tailwind defaults.

## Type roles

One heading family (Display, Geist Variable) and one body family (system stack). Weight 300 is reserved for numerals and display sizes of 28px or more (the ReadinessRing percentage, big metric numbers, the sign-in showcase). Headings never use 300 below 28px.

| Role                                          | Size / line | Weight | Family       | Token and component                                                                            |
| --------------------------------------------- | ----------- | ------ | ------------ | ---------------------------------------------------------------------------------------------- |
| Greeting (dashboard, sign-in, setup headings) | 28 / 36     | 500    | Display      | `--glinr-type-size-greeting`, `<Display size="greeting">`, `PageTemplate titleSize="greeting"` |
| Page title                                    | 20 / 28     | 600    | Display      | `--glinr-type-size-page`, `<Display size="page">`, tight tracking                              |
| Section and card title                        | 15 / 22     | 600    | Display      | `--glinr-type-size-section`, `<Display size="section">`, `CardTitle`, `FormSection`            |
| Banner title                                  | 14 / 20     | 600    | Display      | `--glinr-type-size-banner`, `<Display size="banner">`, `Banner`                                |
| Body                                          | 14 / 20     | 400    | Body         | `text-sm`                                                                                      |
| Secondary                                     | 13          | 400    | Body         | `text-[13px]`, muted foreground                                                                |
| Caption                                       | 12          | 400    | Body         | `text-xs`                                                                                      |
| Eyebrow                                       | 11          | 400    | Mono         | `--glinr-font-eyebrow`, `--glinr-type-size-eyebrow`, uppercase, tracking-widest, `<Eyebrow>`   |
| Numbers                                       | inherits    | 300+   | Mono or body | Apply `--glinr-type-numeric-features` so digits align in tables and meters.                    |

Weights come from `--glinr-type-heading-weight` (600), `--glinr-type-greeting-weight` (500) and `--glinr-display-weight` (300, numerals and display only).

## Page hero

`PageTemplate` renders the hero as one row: tile (`--glinr-hero-tile-size`, 40px at every density), 12px gap, text block, spacer, actions. The eyebrow sits 4px above the title; the title and status badge share a baseline. With a one line description the tile and text block are vertically centered to each other. When the description wraps the tile top-aligns with the title (below the eyebrow) and the actions align to the title row. Pages with a brand mark pass `<BrandTile size="lg">` as `logo`; they never build their own header markup.

Fonts are the `@fontsource-variable/geist` and `@fontsource-variable/geist-mono` packages, latin subset, `font-display: swap`, with the system stack as fallback.

## Elevation scale

| Token                  | Tailwind         | Use                                          |
| ---------------------- | ---------------- | -------------------------------------------- |
| `--glinr-elev-header`  | `shadow-header`  | Sticky header once content scrolls under it. |
| `--glinr-elev-card`    | `shadow-card`    | Resting cards and panels.                    |
| `--glinr-elev-popover` | `shadow-popover` | Menus, selects, popovers.                    |
| `--glinr-elev-dialog`  | `shadow-dialog`  | Modal dialogs and sheets.                    |

Dark mode uses a darker translucent shadow plus a 1px top highlight (`--glinr-highlight`) or ring so depth stays visible. There are no glow or gradient tokens.

## Spacing, radius, motion, widths

- Space: `--glinr-space-1,2,3,4,6,8,12,16` on a 4px rhythm; `--glinr-page-gap` aliases `space-6`.
- Radius: `sm 6px`, `md 8px`, `lg 10px` (the legacy `--glinr-radius`), `xl 14px`.
- Motion: `--glinr-motion-fast` 120ms, `base` 200ms, `slow` 320ms, `--glinr-motion-ease`. Honour `prefers-reduced-motion`.
- Content: `--glinr-content-narrow` 40rem, `--glinr-content-max` 72rem, `--glinr-content-max-wide` 88rem.

## Adding a token

1. Add it to `glinr.tokens.json` in the right group with `$value`, `$description` and `$extensions.glinr.stability` (start `experimental`).
2. Add `$extensions.glinr.tailwind` only if a utility is wanted.
3. Run `npm run tokens` in `web/` and commit the regenerated CSS.
4. Use it as `var(--glinr-...)` or the Tailwind utility. Do not hardcode hex or px in a component when a token exists.

## Changelog

2026-10-12, type scale and hero fix. Page titles 24 to 20px, banner and section titles 18 and 16 to 14 and 15px, all weight 600 in the Display family (previously 300 Geist next to 500 system-ui). Greeting 24/30 to 28px weight 500. Eyebrow 12 to 11px. `--glinr-hero-tile-size-comfortable` now equals `--glinr-hero-tile-size` (2.5rem) so the hero tile no longer changes size with density. New tokens: `type-heading-weight`, `type-greeting-weight`, `type-size-banner|section|page|greeting|eyebrow`, `type-leading-banner|section|page|greeting`.

2026-10-12, token source introduced. Value changes to existing variables:

- `--glinr-font-display`: was `var(--font-sans)` (system stack), now `'Geist Variable'` then the same system stack. Visible on Display headings.
- `--glinr-font-eyebrow`: was `var(--font-mono)`, now `'Geist Mono Variable'` then the same mono stack. Visible on eyebrows.
- `--glinr-page-gap`: same value (1.5rem), now an alias of `--glinr-space-6`.
- `--shadow-header|card|popover|dialog`: same values, now emitted from `glinr-theme.css` instead of `index.css`.

New tokens (additive, none applied to existing components yet): `color-border-strong`, `color-grid-line`, `color-fg`, `color-fg-muted`, `color-fg-subtle`, `color-accent`, `color-showcase-muted`, `color-showcase-rule`, `space-*`, `radius-sm|md|xl`, `content-narrow`, `type-*` scale, leading, tracking, weight and numeric features, `font-body`, `motion-*`.

## GLIN UI

`@glinui/ui` and `@glinui/tokens` (0.1.1, MIT) are installed in `web/`. Only `Skeleton` is used so far, through `web/src/components/ui/skeleton.tsx`, the single adapter. Every existing skeleton (`ListSkeleton`, `TableSkeleton`, settings skeletons) goes through it, so the look changes in one place.

- Variants: the adapter accepts `glinr` (default), `solid`, `plain`, `soft`, `outline`, `ghost`, the names on the docs page. Version 0.1.1 publishes `default | glass | liquid | matte | outline | ghost`, so `glinr` maps to `matte`, `solid` and `plain` to `default`, `soft` to `ghost`. Drop the map once the docs names ship.
- Motion: the shimmer is `motion-reduce:hidden`. The `skeleton-shimmer` keyframes are not shipped by the package, so `src/index.css` defines them.
- Tailwind does not scan `node_modules`, so `src/index.css` lists the Skeleton classes with `@source inline(...)`. A file-path `@source` doubled the compiled stylesheet.
- Bundle: `@glinui/ui` has no `"sideEffects": false`, so its barrel pulls every Radix package. `vite.config.ts` marks the package side-effect free for the production build, which keeps the main chunks flat (vendor +2 KB). Vitest inlines the package because its ESM uses extensionless imports Node cannot load.
- Not installed: `@glinui/cli` is unpublished on npm (404), so the docs' `dlx @glinui/cli add skeleton` fails. This is an issue to fix on the glinui side; use the package route until then.

### Token overlap

`@glinui/tokens/theme.css` is deliberately not imported: the mapped Skeleton variants need none of its variables. It redefines names we already own: `--color-background`, `--color-foreground`, `--color-border`, `--color-accent`, `--color-accent-foreground`, `--radius-sm|md|lg|xl|2xl` (different values, e.g. `--radius-lg: 1rem` against our `var(--radius)`), `--motion-fast`, `--motion-slow`. Importing it plain would change those values app wide. If the glass or liquid variants are adopted, import it as `@import '@glinui/tokens/theme.css' layer(glinui-tokens)` with `@layer glinui-tokens;` declared before `@import 'tailwindcss'`, and re-check the overlap above.
