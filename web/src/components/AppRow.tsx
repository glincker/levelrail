import { Link } from '@tanstack/react-router'
import { TemplateLogo } from './TemplateLogo'
import { logoIdForImage } from '../lib/imageLogo'
import { PackageIcon, GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import { AppRowActions } from './AppRowActions'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import type { AppListEntry, AppStatusSummary } from '../types/appDetail'
import { STATUS_DOT_COLOR } from '../lib/appStatus'

// Shared column grid between the sticky header (routes/apps/index.tsx)
// and every row below (AppRow, RowSkeleton), so header labels line up
// with row content pixel-for-pixel without either file hardcoding the
// same width numbers twice. Icon avatar, name, image, domain, port,
// trailing chevron, in that order.
export const APP_LIST_GRID =
  'grid grid-cols-[1.25rem_2rem_minmax(0,1.5fr)_minmax(0,1.1fr)_minmax(0,1.3fr)_4.5rem_2rem] items-center gap-3'

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
export function AppRow({
  app,
  selected = false,
  onSelect,
}: {
  app: AppListEntry
  selected?: boolean
  onSelect?: (name: string, checked: boolean) => void
}) {
  const domain = app.domains?.[0] ?? null
  const extraDomains = (app.domains?.length ?? 0) - 1

  return (
    <div
      className={`${APP_LIST_GRID} relative h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <span className="relative z-10 flex items-center">
        <Checkbox
          checked={selected}
          aria-label={`Select ${app.name}`}
          onCheckedChange={(checked) => {
            onSelect?.(app.name, checked)
          }}
        />
      </span>

      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <TemplateLogo
          id={logoIdForImage(app.image) ?? ''}
          className="size-4"
          fallback={<PackageIcon className="size-4" aria-hidden="true" />}
        />
      </span>

      <span className="flex min-w-0 flex-col justify-center gap-0.5">
        <span className="flex min-w-0 items-center gap-2 text-sm font-medium text-foreground">
          <StatusDot status={app.status} />
          <Link
            to="/apps/$name"
            params={{ name: app.name }}
            className="truncate after:absolute after:inset-0"
          >
            {app.name}
          </Link>
        </span>
        {(app.tags && app.tags.length > 0) || app.environment_name ? (
          <span className="flex flex-wrap items-center gap-1 pl-4">
            {app.environment_name ? (
              <Badge variant="muted" className="px-1.5 py-0 text-[10px]">
                {app.environment_name}
              </Badge>
            ) : null}
            {(app.tags ?? []).slice(0, 3).map((tag) => (
              <Badge
                key={tag}
                variant="outline"
                className="px-1.5 py-0 text-[10px] text-muted-foreground"
              >
                {tag}
              </Badge>
            ))}
            {(app.tags?.length ?? 0) > 3 ? (
              <Badge variant="muted" className="px-1.5 py-0 text-[10px]">
                +{(app.tags?.length ?? 0) - 3}
              </Badge>
            ) : null}
          </span>
        ) : null}
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

      <AppRowActions app={app} />
    </div>
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
      <div className="size-4 animate-pulse rounded bg-muted" />
      <div className="size-8 animate-pulse rounded-md bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="h-4 w-40 animate-pulse rounded bg-muted" />
      <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      <div className="h-4 w-10 animate-pulse justify-self-end rounded bg-muted" />
      <div className="h-4 w-4 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
