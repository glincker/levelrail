# ADR 053: thesvg UI icons replace Phosphor for new dashboard code

Status: Accepted

Update 2026-10-29: the migration is finished. No Phosphor import remains and the
`@phosphor-icons/react` dependency is removed.

Date: 2026-10-29

Supersedes: ADR 014

## Context

ADR 014 made Phosphor the only UI icon set in `/web`. We now ship our own UI
icon set, `@thesvg/ui-react`, which is already a dependency and used by recent
components such as AppResourceTree. Keeping two sets as the rule means new code
has no clear answer and Phosphor never goes away.

## Decision

New dashboard code uses `@thesvg/ui-react` for UI icons. No new
`@phosphor-icons/*` or `lucide-react` imports are added. Brand marks still go
through `BrandIcon` (`@thesvg/react`).

## Migration approach

Opportunistic, no mass rewrite. When a file is touched for other work, its
Phosphor icons are migrated in the same change. Existing Phosphor imports in
untouched files stay as they are. `@phosphor-icons/react` is removed from
`web/package.json` once no import remains.

## Guard behavior

The pre-commit stage inspects `git diff --cached -U0` and fails only when an
added line imports from `@phosphor-icons/` or `lucide-react`. Existing imports
on untouched lines never fail. A file that already imports Phosphor still fails
if the change adds another Phosphor import line. `scripts/check-icon-rules.sh`
keeps the logo and second-library rules.

## Consequences

- One icon source for new code, owned by us.
- Mixed icon sets in the dashboard until migration finishes.
- A file that is edited and keeps a Phosphor import is not forced to migrate,
  but cannot grow its Phosphor usage.
