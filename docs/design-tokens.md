---
title: GLINR design tokens
description: The single source of truth for color, space, radius, elevation, type and motion in the Levelrail dashboard, and how to add or change a token.
---

# Design tokens

The dashboard theme lives in `web/src/styles/glinr-tokens.css`, labelled "GLINR CSS tokens". Every variable is namespaced `--glinr-*` and defined for light and dark.

- **Source**: the TheAuth visuals used by the sign-in showcase (black stage, `#0e0f11` card, `#cfcfd3` text, violet, teal and lime glow).
- **Mapping**: `index.css` points shadcn tokens such as `--radius` at `--glinr-*` only where the values already match, so the app is not repainted.
- **Elevation**: `--glinr-elev-header`, `-card`, `-popover` and `-dialog` are exposed as Tailwind `shadow-header`, `shadow-card`, `shadow-popover` and `shadow-dialog`. In dark mode they use a darker translucent shadow plus a faint 1px top highlight or border so depth stays visible.
- **Sticky header**: the top bar stays pinned while content scrolls and gains `shadow-header` once content is under it.

Use these tokens in components instead of per component colors or shadows.
