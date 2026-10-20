# ADR 022: Two brand colors, not one: petrol-blue identity, amber action accent

Status: Accepted

Date: 2026-09-30

## Context

`brand.yaml`'s `primary_color` was locked to petrol-blue (`#107292`) during
the visual identity work, checked against every direct competitor's real
brand hex (Railway/Coolify/Fly.io/Zeabur's purple cluster, Supabase/Linode's
green, DigitalOcean/Dokku's blue, Netlify's near-identical teal).

Separately, and before that decision, `docs/.vitepress/theme/custom.css`
already had its own "rail signal" theme: an amber accent (`#d97706`/`#fbbf24`
family) used as the interactive color across the whole public docs and
marketing site, links, buttons, hero glow, badges, hover states. That choice
had its own, different reasoning: distinct from Railway's violet and
Coolify's teal/blue, and amber literally nods at a real rail-signal light,
a deliberate tie to the product's own name.

These two decisions were made independently and never reconciled. Once the
mark itself turned blue, the docs site reads as "orange everywhere, blue
only in the tiny logo," which looks accidental rather than intentional,
exactly the kind of thing a visitor (or a future agent) would try to "fix"
by guessing.

Before deciding anything, checked how far amber actually spreads:

- `primary_color` is not consumed as a CSS or UI color anywhere today, it is
  a Go struct field (`internal/brand`) referenced only in frontend test
  mocks as a placeholder value.
- The dashboard's (`web/`) own shadcn `--primary` token is plain neutral
  grayscale (`oklch(0.205 0 0)`, zero chroma), not amber.
- The large number of amber references inside dashboard components
  (`AlertRulesPanel`, `StatusChip`, `AppHealthTimeline`, attention strips,
  log-level chips, etc.) are semantic warning-state color, the standard
  red/amber/green severity convention, a different and legitimate use of
  the hue that has nothing to do with brand accent and must not be
  confused with it.

So the actual conflict is narrow: the docs/marketing site's chosen
interactive accent versus the product mark's locked identity color,
nothing wider, and nothing semantic is involved.

## Decision

Run two official brand colors with explicit, separate roles instead of
collapsing to one:

- **Petrol-blue (`#107292` family): identity color.** The mark itself
  (favicon, dashboard sidebar glyph, docs navbar icon) and anywhere
  Levelrail's own shape/logo appears. Never used as a page-wide interactive
  accent.
- **Amber (`#d97706`/`#fbbf24` family, the docs site's "rail signal"
  theme): action/interactive accent for the public docs and marketing site
  specifically.** Links, buttons, hero highlights, badges, hover states.
  Kept because it was independently checked against competitor colors, and
  because it is deliberate brand storytelling, not a leftover placeholder.
- The dashboard (`web/`) keeps its current neutral/grayscale shadcn
  `--primary` and continues to use amber/orange strictly as semantic
  warning-state color, never as a brand accent. This ADR does not introduce
  a brand accent into the dashboard's own UI chrome.

## Rejected alternatives

- **Retheme the docs site's accent to petrol-blue for full consistency.**
  Rejected: real surgery across `custom.css`, every link, button, hero
  gradient, badge, hover state, for a cohesion gain a documented two-color
  system achieves more cheaply, and it would throw away the rail-signal
  color metaphor that gives the docs site's "amber on cool slate"
  personality its own reason to exist.
- **Wire petrol-blue into the dashboard's shadcn `--primary` token now, to
  tie the actual product UI to the mark.** Considered, but out of scope
  here: a separate, real design decision touching every primary button and
  focus ring across the whole authenticated app, deserving its own sign-off
  rather than riding in on a documentation fix. Left for a future ADR if
  the team wants to pursue it.
- **Leave the inconsistency undocumented, as a known quirk.** Rejected:
  undocumented is exactly what reads as accidental to anyone who notices,
  as happened here, and the fix is cheap: write the rule down.

## Consequences

- No code or CSS changes. This ADR only records the intended role of each
  color so the next person, or agent, touching brand colors does not try to
  "fix" the amber/blue split by guessing.
- `brand.yaml` gets a short comment pointing at this ADR, next to
  `primary_color`.
- If the dashboard ever adopts a brand accent of its own, that decision
  supersedes this ADR's "dashboard stays neutral" clause via its own ADR,
  not a silent edit here.
