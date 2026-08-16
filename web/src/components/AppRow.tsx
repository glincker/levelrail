import { Link } from '@tanstack/react-router'
import {
  PackageIcon,
  CaretRightIcon,
  GlobeIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { AppListEntry, AppStatusSummary } from '../types/appDetail'
import { STATUS_DOT_COLOR } from '../lib/appStatus'

// Shared column grid between the sticky header (routes/apps/index.tsx)
// and every row below (AppRow, RowSkeleton), so header labels line up
// with row content pixel-for-pixel without either file hardcoding the
// same width numbers twice. Icon avatar, name, image, domain, port,
// trailing chevron, in that order.
export const APP_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_minmax(0,1.1fr)_minmax(0,1.3fr)_4.5rem_1rem] items-center gap-3'

// Links by app.name, not a separate id: the detail route
// (routes/apps/$name.tsx) and its backing API (GET /api/v1/apps/{name})
// both key off the app's name, the only identifier appResource actually
// carries.
//
// The status dot next to the name is backed by GET /api/v1/apps'
// batched status field (internal/api/apps.go's appListResource /
// summarizeAppConditions), not a per-row fetch: one query across every
// listed app's conditions, computed server-side, so this list stays
// cheap past 50+ rows the same way the rest of this component already
// does (per the project's virtualization requirement). The app detail
// route's ConditionsPanel remains where the full reconcile detail
// lives; this dot is just the same category/color StatusDot below
// summarizes, matching web/src/lib/appStatus.ts's summarizeAppStatus.
export function AppRow({ app }: { app: AppListEntry }) {
  const domain = app.domains?.[0] ?? null
  const extraDomains = (app.domains?.length ?? 0) - 1

  return (
    <Link
      to="/apps/$name"
      params={{ name: app.name }}
      className={`${APP_LIST_GRID} h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <PackageIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="flex min-w-0 items-center gap-2 text-sm font-medium text-foreground">
        <StatusDot status={app.status} />
        <span className="truncate">{app.name}</span>
      </span>

      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {app.image}
      </span>

      <span className="flex min-w-0 items-center gap-1.5">
        {domain ? (
          <>
            <GlobeIcon
              className="size-3.5 shrink-0 text-muted-foreground/70"
              aria-hidden="true"
            />
            <span className="truncate text-xs text-muted-foreground">
              {domain}
            </span>
            {extraDomains > 0 ? (
              <Badge variant="muted" className="shrink-0 px-1.5 text-[11px]">
                +{extraDomains}
              </Badge>
            ) : null}
          </>
        ) : (
          <span className="text-xs text-muted-foreground/60 italic">
            no domain
          </span>
        )}
      </span>

      <Badge
        variant="outline"
        className="justify-self-end font-mono text-[11px] text-muted-foreground"
      >
        :{app.port}
      </Badge>

      <CaretRightIcon
        className="size-4 shrink-0 justify-self-end text-muted-foreground/50"
        aria-hidden="true"
      />
    </Link>
  )
}

// title gives mouse users the label as a tooltip; role="img" +
// aria-label carries the same text to screen readers, since the dot's
// color alone conveys nothing without it.
export function StatusDot({ status }: { status: AppStatusSummary }) {
  return (
    <span
      className={`size-2 shrink-0 rounded-full ${STATUS_DOT_COLOR[status.variant]}`}
      role="img"
      aria-label={status.label}
      title={status.label}
    />
  )
}

// Currently unused by the list component's happy path: the route loader
// (routes/apps/index.tsx) awaits appListQueryOptions() before the
// component ever mounts, via useSuspenseQuery, so there is no in-place
// loading state to fill once AppListPage renders. It backs the route's
// pendingComponent instead, for the slow-network case where the loader
// itself hasn't resolved yet and the router shows a pending fallback,
// matching APP_LIST_GRID exactly so the skeleton doesn't jump when real
// rows swap in.
export function RowSkeleton() {
  return (
    <div
      className={`${APP_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-8 animate-pulse rounded-md bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="h-4 w-40 animate-pulse rounded bg-muted" />
      <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      <div className="h-4 w-10 animate-pulse justify-self-end rounded bg-muted" />
      <div className="h-4 w-4 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
