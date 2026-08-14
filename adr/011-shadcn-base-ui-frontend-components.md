# ADR 011: shadcn/ui with Base UI as `/web`'s component foundation, theauth SDK skipped for login

Status: Accepted

Date: 2026-08-12

## Context

The frontend architecture decision locks the frontend stack: React, Vite, TypeScript, Tailwind,
embedded into the Go binary as static assets, route-level code splitting
from the start ("the dashboard should not ship the log viewer's bundle").
It also names the specific failure mode to avoid: Coolify's
Livewire dashboard, which "degrades with resource count," reflecting the
project's framing of why Levelrail exists at all. Neither of these is negotiable
context for any decision about what `/web`'s component library looks like.

Levelrail is one of several GLINCKER products, and two of the others were
real candidates for reuse rather than building `/web`'s UI from scratch:
`thesvg` (an icon and component library, Next.js 16 App Router, static
export to Vercel) and `theauth` (a JS/TS auth SDK: `packages/client`,
`packages/react`, `packages/ui`, built around a theauth-go backend).
The project's own Phase 4 work list names both explicitly: "Integration of
first-party GLINCKER libraries: theauth for the auth layer, thesvg for the
icon system," deferred to Phase 4 "so the platform's own auth is boring
and proven first." The Dashboard & auth work pass (TASKS.md, started
2026-08-12) needed a login screen and a component foundation now, in
Phase 1, ahead of that deferred integration point, so the reuse question
had to be answered early rather than assumed away.

Four research streams ran in parallel on 2026-08-12 to settle this and
related dashboard/auth questions; `docs-local/research/frontend-component-
reuse.md` is the one that covers thesvg's component system and theauth's
SDK. It was read in full before this decision. Its recommendation was
adopted and implemented before this ADR was written, which is the gap
this ADR closes: the project's process requires "one ADR per architectural
decision... with the rejected alternatives written down," and this
decision (a new component library, a new primitive dependency, a new
`web/package.json` diff, and an explicit choice to not integrate an
existing GLINCKER SDK) qualifies.

## Decision

**(a) shadcn/ui, adopted incrementally via a fresh `npx shadcn@latest init
-t vite -b base -p nova` run directly against `web/`, with Base UI
(`@base-ui/react`) as the primitive backend.** Not a copy of thesvg's
`components.json`. thesvg's config is tuned for its own Next.js/RSC/
static-export shape (`"rsc": true`, aliases pointing at `src/app`) and
only ships 13 of the roughly 25-30 components a dashboard needs; a fresh
Vite-targeted init auto-detects the bundler, sets `rsc: false` and correct
aliases, and costs nothing extra since it's the same shadcn registry
either way (`frontend-component-reuse.md` section 1.5). The resulting
`web/components.json` records `"style": "base-nova"`, and `web/
package.json`'s `@base-ui/react ^1.7.0` dependency (not
`@radix-ui/react-*`) confirms Base UI landed as the primitive backend,
per TASKS.md's "shadcn/ui setup" entry. 14 files generated into `web/src/
components/ui/`: `button`, `input`, `label`, `card`, `table`, `dialog`,
`select`, `checkbox`, `switch`, `alert`, `popover`, `tabs`, `separator`,
`field` (the last replacing the classic `form` RHF wrapper the research
doc expected, since the current shadcn registry ships `form` as an empty
stub and `field` as its form-library-agnostic successor).

Adoption is incremental, not a rewrite: shadcn components are plain files
dropped into `web/src/components/ui/`. New work uses them (the login
screen this work pass built, Phase 2.4's metrics dashboard later);
existing hand-rolled components (`DomainEditor.tsx`, `EnvEditor.tsx`, and
the rest of `web/src/components/`) migrate opportunistically when touched
for other reasons, not as a dedicated migration task. TASKS.md's own
verification confirms this held in practice: `git diff --stat` showed no
file under `web/src/components/*.tsx` or `web/src/routes/**` touched by
the init itself.

**(b) Skip the theauth SDK for the login and register screens. Hand-roll
a form plus `fetch` against Levelrail's own `POST /api/v1/auth/login` and
`POST /api/v1/auth/register` instead.** Implemented as `web/src/routes/
login.tsx` plus `web/src/components/LoginForm.tsx` and `RegisterForm.tsx`,
styled with the shadcn primitives from part (a) (`Card`, `Tabs`, `Input`,
`Label`/`Field`, `Button`, `Alert`), cookie-based session (same-origin
`fetch` already sends the `session_token` cookie, no `credentials:
'include'` needed), per TASKS.md's "Login screen" entry.

## Rejected alternatives

- **Reusing thesvg's `components.json` directly, or copying its component
  set as-is.** `frontend-component-reuse.md` section 1.1 confirms by
  reading the file directly that thesvg's config is Next-tuned (`"rsc":
  true`, `aliases` pointing at `src/app`) and its `src/components/ui/`
  directory holds only 13 files curated for thesvg's own icon-browsing UI
  (a command palette, an icon-detail sheet), missing `form`, `select`,
  `checkbox`/`switch`, `card`, `alert`, `table`, `toast`, `popover`,
  `accordion`, `label`, all of which a dashboard needs. Section 1.3
  confirms thesvg's app shell itself (Next.js 16 App Router, `output:
  "export"` static export to Vercel) is architecturally unrelated to
  Levelrail's Go-embedded SPA and not reusable at all; what's portable is
  narrower than "reuse thesvg's frontend", it's specifically `src/
  components/ui/*.tsx` and `src/lib/utils.ts`'s `cn()` helper, and even
  those are better regenerated fresh against Vite than adapted from a
  Next-shaped config, since a fresh `init` is less work and less risk for
  an identical result.

- **A big-bang rewrite of existing hand-rolled components to shadcn on
  landing.** Rejected because the project's frontend architecture decision
  and engineering process both favor bounded,
  reviewable slices over sweeping rewrites, and because nothing about
  shadcn/ui's own shape requires it: shadcn components are plain function
  components with the same bundling behavior as `DomainEditor.tsx` today
  (`frontend-component-reuse.md` section 3), so there is no technical
  forcing function to migrate anything that isn't already being touched.
  TASKS.md's shadcn/ui setup entry confirms no existing component file was
  touched by the init.

- **Using the theauth SDK (`@glinr/theauth-react`'s `useSignIn` plus
  `packages/ui`'s `<SignIn>`) for the login screen.** Rejected on a
  backend-shape mismatch, not a code-quality judgment: `frontend-
  component-reuse.md` section 2.2 traces `<SignIn>` through `useSignIn`
  to a `basePath` (default `/api/kavach`) expecting theauth-go's
  contract, OAuth 2.1, magic-link send endpoints, `{success, error}`
  result shapes, and documents theauth's own internal documentation describing
  itself as agent-first ("`AgentIdentity` is primary entity, not `User`").
  Levelrail's actual backend (`internal/api/auth.go`) is bcrypt-plus-
  session-cookie for a single admin user, explicitly "no teams, no RBAC
  yet." Using the SDK against that backend meant either standing up
  theauth-go as Levelrail's auth backend (a separate, larger decision the
  parallel theauth-go fit-assessment work evaluated independently and
  rejected for Phase 1, per TASKS.md's "Don't adopt theauth-go now" entry)
  or reimplementing `useSignIn`'s context contract by hand, at which point
  the SDK import is dead weight around what section 2.4 measured as an
  8-line `fetch` call. The strongest evidence cited is concrete, not
  abstract: theauth's own internal admin dashboard
  (`theauth/sdk/packages/dashboard`, a Vite + React SPA, the closest
  structural sibling to `/web` anywhere in the GLINCKER workspace) does
  not import `@glinr/theauth-react`, `@glinr/theauth-client`, or
  `@glinr/theauth-ui` for its own login screen. `packages/dashboard/src/
  components/login.tsx` hand-rolls a raw `fetch()` against `${apiUrl}/api/
  dashboard/auth` with a bearer-token header and `sessionStorage`
  persistence, no SDK dependency at all. The team that built the theauth
  SDK independently reached the same conclusion for the nearest comparable
  case: a single-secret internal dashboard gate is not what the SDK is
  for.

## Consequences

- New runtime dependencies landed in `web/package.json`: `@base-ui/react`
  (`^1.7.0`), `class-variance-authority` (`^0.7.1`), `clsx` (`^2.1.1`),
  `lucide-react` (`^1.31.0`), `shadcn` (`^4.17.0`, kept as a runtime
  dependency rather than a dev dependency because `web/src/index.css`
  imports `shadcn/tailwind.css` at build time), `tailwind-merge`
  (`^3.6.0`), and `tw-animate-css` (`^1.4.0`). None of these existed in
  `web/package.json` before this work. The root GLINRV5 project's
  file-boundaries rule requires approval for `package.json` changes and
  this repo has no override of that rule; per TASKS.md's shadcn/ui setup
  entry, this diff was explicitly flagged for founder sign-off before
  merge, and it was obtained.
- Choosing Base UI over classic Radix as the primitive backend was a real,
  named open call in the research doc (section 1.5), resolved in favor of
  Base UI specifically because it matches thesvg's own primitive choice
  (confirmed by reading thesvg's `button.tsx` and `dialog.tsx` source
  directly, both importing from `@base-ui/react`, not `@radix-ui/react-*`).
  This buys GLINCKER visual and behavioral consistency across products
  without directly reusing thesvg's code: the same primitive layer under
  two independently-generated component sets, rather than a shared
  dependency or a forked config.
- Two component systems coexist in `web/src/components/` during the
  incremental migration period: shadcn primitives under `components/ui/`
  for new work, and hand-rolled components (`DomainEditor.tsx`,
  `EnvEditor.tsx`, and others) for everything not yet touched. This is an
  accepted, deliberate tradeoff rather than an oversight, but it means
  contributors need to know which convention applies where until the
  migration completes; there is no single "the dashboard's component
  style" answer during this period, only "shadcn for anything new, ask
  before assuming an existing file's pattern is still the target."
- The login screen's own shape sets the precedent this ADR's part (b)
  establishes for any future GLINCKER-SDK reuse question in `/web`: fit is
  evaluated against Levelrail's actual backend contract, not against how
  polished or well-built the SDK's own components are. TASKS.md's "Don't
  adopt theauth-go now" entry already names the reconsideration trigger:
  if theauth-go is adopted as Levelrail's backend auth service at Phase 4
  when teams/RBAC are in scope, `packages/ui`/`packages/react` become the
  matching frontend half and this specific rejection should be revisited,
  not treated as permanent.
