# web

The dashboard for the control plane. It is embedded into the Go binary with `embed.FS`, so the server needs no Node runtime and no separately hosted frontend.

## Stack

React 19, Vite, TypeScript (strict), Tailwind CSS, TanStack Router (file-based routes in `src/routes`, split per route), TanStack Query and TanStack Virtual. UI components come from `@glinui/ui`.

## Run, test, build

```
npm install                 # install dependencies
npm run dev                 # Vite dev server
npm test                    # vitest run, component and hook tests under src/
npm run test:watch          # vitest in watch mode
npm run typecheck           # tsc -b only
npm run lint                # eslint (lint:fix to fix)
npm run format:check        # prettier --check (format to write)
npm run build               # production build in dist/
npm run check-bundle-size   # compare chunk sizes with bundle-budgets.json
```

CI runs tests, type checks and lint in the "Web (tsc, eslint)" job whenever `web/` changes. `npm run build` also writes `dist/stats.html`, a treemap of chunk sizes.

## Conventions

- No data fetching in component bodies. Route loaders prime the Query cache and components read it with `useSuspenseQuery` or `useSuspenseInfiniteQuery` on the same key.
- Query keys come from a factory per resource (see `src/queries/apps.ts`), never an inline array.
- Lists that can exceed 50 items are virtualized with `@tanstack/react-virtual`.
- No `any`. `src/routeTree.gen.ts` is generated on every `dev` and `build` run and is not committed.

## i18n and icons

User-facing text goes through i18n, with files under `src/locales` and setup in `src/i18n`. Brand marks come from `@thesvg/react` and UI icons from `@thesvg/ui-react`. The checks and how to run them are in [Contributing guards](../docs/contributing-guards.md).
