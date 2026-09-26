import { Link } from '@tanstack/react-router'
import { AppRowActions } from './AppRowActions'
import { Checkbox } from '@/components/ui/checkbox'
import type { AppListEntry, AppStatusSummary } from '../types/appDetail'
import { STATUS_DOT_COLOR } from '../lib/appStatus'
import { statusBucket } from '../lib/appsListView'
import { AppLogo, AppMetaChips } from './apps/AppMetaChips'
import {
  ErrorCell,
  LastDeployCell,
  P95Cell,
  TrafficSpark,
} from './apps/AppMetricCells'
import { AppQuickActions } from './apps/AppQuickActions'
import { useAppRowMetrics } from './apps/useAppRowMetrics'

// Shared column grid for the sticky header (routes/apps/index.tsx) and
// every row. Traffic columns only exist from lg up.
export const APP_LIST_GRID =
  'grid grid-cols-[1.25rem_2rem_minmax(0,1fr)_5rem] lg:grid-cols-[1.25rem_2rem_minmax(0,1.6fr)_6rem_4.5rem_4rem_5rem_6.5rem] items-center gap-3'

export function AppRow({
  app,
  selected = false,
  onSelect,
}: {
  app: AppListEntry
  selected?: boolean
  onSelect?: (name: string, checked: boolean) => void
}) {
  const metrics = useAppRowMetrics(app.name)
  return (
    <div
      className={`${APP_LIST_GRID} group/row relative h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
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

      <AppLogo image={app.image} />

      <span className="flex min-w-0 flex-col justify-center gap-1">
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
        <span className="pl-4">
          <AppMetaChips app={app} />
        </span>
      </span>

      <span className="hidden lg:block">
        <TrafficSpark metrics={metrics} name={app.name} />
      </span>
      <span className="hidden lg:block">
        <P95Cell metrics={metrics} />
      </span>
      <span className="hidden lg:block">
        <ErrorCell metrics={metrics} />
      </span>
      <span className="hidden lg:block">
        <LastDeployCell metrics={metrics} />
      </span>

      <span className="flex items-center justify-self-end">
        <AppQuickActions app={app} />
        <AppRowActions app={app} />
      </span>
    </div>
  )
}

// title gives mouse users the label as a tooltip; role="img" +
// aria-label carries the same text to screen readers.
export function StatusDot({ status }: { status: AppStatusSummary }) {
  const deploying = statusBucket({ status }) === 'deploying'
  return (
    <span
      className={`relative size-2 shrink-0 rounded-full ${deploying ? 'bg-amber-500' : STATUS_DOT_COLOR[status.variant]}`}
      role="img"
      aria-label={status.label}
      title={status.label}
    >
      {deploying ? (
        <span
          aria-hidden="true"
          className="absolute inset-0 animate-ping rounded-full bg-amber-500/70 motion-reduce:animate-none"
        />
      ) : null}
    </span>
  )
}

// Backs the route's pendingComponent so the skeleton matches
// APP_LIST_GRID and does not jump when real rows swap in.
export function RowSkeleton() {
  return (
    <div
      className={`${APP_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-4 animate-pulse rounded bg-muted" />
      <div className="size-8 animate-pulse rounded-lg bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="hidden h-4 w-20 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-12 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-10 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-14 animate-pulse rounded bg-muted lg:block" />
      <div className="h-4 w-8 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
