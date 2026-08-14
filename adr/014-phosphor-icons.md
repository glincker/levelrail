# ADR 014: Phosphor icons replace lucide-react in /web

Status: Accepted

Date: 2026-08-13

## Context

Every repo in the organization inherits a standing, platform-wide engineering
rule: "No new icon sets (Phosphor only - `@phosphor-icons/react/dist/ssr`)".
That rule predates this repo and is not something an individual ADR gets to
relitigate.

ADR 011 (shadcn/ui with Base UI as `/web`'s component foundation) recorded
`lucide-react` (`^1.31.0`) in its Consequences section as a dependency that
landed in `web/package.json`, but its own text is explicit about how: it
arrived as a side effect of running `npx shadcn@latest init`, which pulls in
lucide-react as shadcn's default icon dependency, not as a deliberate icon
library choice anyone evaluated against the Phosphor rule. ADR 011's own
"Consequences" list documents every new dependency that init run introduced
and flags the whole diff for founder sign-off; it does not single out
lucide-react as a considered decision, because it was not one. Once
`lucide-react` imports existed, ordinary feature work generated 44 files
importing from it: 39 application components and routes, plus the 5
shadcn-generated primitives under `web/src/components/ui/` (`dialog.tsx`,
`select.tsx`, `sheet.tsx`, `sidebar.tsx`, `checkbox.tsx`) that shadcn's init
wrote directly.

This is exactly the situation the platform-wide icon rule exists to prevent:
a second icon set entering the codebase unreviewed. The user explicitly
approved fixing it now, including the `web/package.json` dependency swap.

## Decision

Swap `lucide-react` for `@phosphor-icons/react` everywhere in `/web`, using
the `@phosphor-icons/react/dist/ssr` subpath for every icon component import,
matching the platform rule's own wording exactly. `web/package.json` drops
`lucide-react` (`^1.31.0`) and adds `@phosphor-icons/react` (`^2.1.10`).

Lucide and Phosphor do not share icon names. Every one of the 44 files was
read and edited individually rather than swapped by a blind find/replace,
and every replacement name was confirmed against the installed package's
actual TypeScript exports (`node_modules/@phosphor-icons/react/dist/ssr/
*.d.ts`) before use, not guessed from memory. Many names carried over with
only the Lucide-specific suffix convention removed (e.g. `PlusIcon`,
`GlobeIcon`, `DatabaseIcon`, `CheckIcon`, `XIcon` exist under both libraries
with identical names). The names that do not are:

| Lucide name | Phosphor name | Notes |
| --- | --- | --- |
| `TriangleAlertIcon` / `TriangleAlert` | `WarningIcon` | Phosphor has no `TriangleAlert`; `Warning` is the triangle-with-exclamation glyph. |
| `ServerIcon` | `HardDrivesIcon` | Phosphor has no literal "server" icon; stacked hard drives is the closest semantic match, used for node/machine rows. |
| `Trash2Icon` / `Trash2` | `TrashIcon` | Phosphor ships one trash-can glyph, not a numbered variant. |
| `HeartPulseIcon` | `HeartbeatIcon` | |
| `CheckCircle2Icon` | `CheckCircleIcon` | Phosphor has one check-in-circle glyph, not a numbered variant. |
| `CircleAlertIcon` | `WarningCircleIcon` | |
| `HelpCircleIcon` | `QuestionIcon` | Phosphor's `Question` is the circle-bordered question mark; a separate bare-glyph `QuestionMark` also exists and was not used. |
| `BoxIcon` | `PackageIcon` | Used for the single-app icon; a deployable app reads more naturally as a package than a bare cube. |
| `BoxesIcon` | `StackIcon` | Used for the sidebar's "Apps" nav icon (multiple apps). |
| `ChevronRightIcon` / `ChevronDownIcon` / `ChevronUpIcon` | `CaretRightIcon` / `CaretDownIcon` / `CaretUpIcon` | Phosphor's chevron-shaped glyphs are named "Caret", not "Chevron". |
| `Activity` / `ActivityIcon` | `PulseIcon` | Phosphor's `Pulse` (an EKG-style line) is the closest visual match to Lucide's `Activity`. |
| `RefreshCw` | `ArrowsClockwiseIcon` | |
| `RotateCcw` | `ArrowCounterClockwiseIcon` | |
| `Search` | `MagnifyingGlassIcon` | |
| `X` | `XIcon` | Same glyph, Phosphor only exports the `Icon`-suffixed name. |
| `BellRing` | `BellRingingIcon` | |
| `EyeOffIcon` | `EyeSlashIcon` | |
| `PanelLeftIcon` | `SidebarSimpleIcon` | Used for the shadcn sidebar's collapse/expand trigger; `SidebarSimple` is a literal sidebar-layout glyph, a closer match than any panel icon. |
| `VariableIcon` | `BracketsCurlyIcon` | Phosphor has no algebraic-variable glyph; curly braces read as "config/env values", the nearest available metaphor for an env-var editor. |
| `LayoutDashboardIcon` | `SquaresFourIcon` | Phosphor has no literal dashboard glyph; a four-square grid is the conventional dashboard substitute. |
| `FlaskConicalIcon` | `FlaskIcon` | |
| `LifeBuoyIcon` | `LifebuoyIcon` | Casing only. |
| `SparklesIcon` | `SparkleIcon` | Singular in Phosphor. |
| `ScrollTextIcon` | `ScrollIcon` | |
| `KeyRoundIcon` | `KeyIcon` | Phosphor's single `Key` glyph is already the rounded-bow style Lucide splits into `Key`/`KeyRound`. |
| `LogOutIcon` | `SignOutIcon` | |
| `SettingsIcon` | `GearIcon` | |
| `type LucideIcon` | `type Icon` | Component-type import, used where an icon is picked at runtime from a lookup table (`ConditionsPanel.tsx`, `ThemeToggle.tsx`). Imported from `@phosphor-icons/react` (the root package, not the `/dist/ssr` subpath) since the SSR subpath's `index.d.ts` does not re-export the shared `Icon` type; being a type-only import it is erased at build time regardless of subpath, so this does not pull any extra runtime code into the SSR-only bundle. |

Every existing usage's `className` (Tailwind sizing like `size-4`,
`h-4 w-4`, color utilities) and other JSX props carried over unchanged:
Phosphor icon components accept the same `className`-driven sizing pattern
as Lucide's, so no visual-sizing adjustment was needed beyond the import and
component-name swap. Verified visually in a running dev server: sidebar nav
icons, a dialog's close icon and title icon, a `select`'s caret and
checkmark, a `checkbox`'s check mark, and the Security/General settings
pages' several distinct icons all render as real glyphs, not blank boxes or
broken imports.

## Rejected alternatives

- **Leaving `lucide-react` in place and treating ADR 011's line item as a
  closed, past decision.** Rejected because the platform-wide Phosphor rule
  is not scoped to "new features going forward", it is a standing rule this
  repo has no override for (per its own File Boundaries section deferring to
  the organization's platform-wide engineering guide where a repo doesn't
  define its own), and 44 files
  importing a second icon set is the exact failure mode the rule exists to
  catch, whether or not the introduction was deliberate.
- **A blind find-and-replace of import specifiers only, leaving Lucide icon
  names in JSX and letting TypeScript catch the fallout.** Rejected because
  the two libraries' naming conventions diverge enough (see the table above)
  that this would have produced a large batch of broken builds to chase down
  one compiler error at a time, with no guarantee every visual regression
  (e.g. a semantically wrong substitute icon) would even surface as a type
  error. Reading and editing each file, then confirming every replacement
  name against the installed package's real exports, was slower but
  produced a working build and a defensible mapping on the first pass.

## Consequences

- Supersedes ADR 011's Consequences-section line item listing `lucide-react`
  (`^1.31.0`) as a dependency that landed. Every other part of ADR 011 (Base
  UI, shadcn, the login-screen SDK decision) is unaffected and still stands.
- `web/package.json` changed: `lucide-react` removed, `@phosphor-icons/react`
  (`^2.1.10`) added. This is a dependency swap already approved by the user
  for this specific change, per the platform-wide file-boundaries rule
  requiring approval for `package.json` edits.
- The 5 shadcn-generated primitives under `web/src/components/ui/`
  (`dialog.tsx`, `select.tsx`, `sheet.tsx`, `sidebar.tsx`, `checkbox.tsx`)
  now import from `@phosphor-icons/react/dist/ssr` like every other file in
  `/web`. Any future `npx shadcn@latest add <component>` run will
  regenerate a file with a fresh `lucide-react` import; whoever runs that
  command needs to swap it by hand (or re-run this ADR's mapping) rather
  than assume shadcn's own defaults already match this repo's icon library.
- No product-name strings were introduced by this change (this repo's own
  brand-indirection rule, section 3, is unaffected, this is a pure
  icon-library swap).
