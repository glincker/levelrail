import { GridFourIcon, RowsIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import {
  STATUS_BUCKETS,
  type StatusBucket,
  type StatusFilter,
  type ViewMode,
} from '../../lib/appsListView'

const CHIP: Record<StatusBucket, { label: string; dot: string }> = {
  running: { label: 'Running', dot: 'bg-emerald-500' },
  deploying: { label: 'Deploying', dot: 'bg-amber-500' },
  failing: { label: 'Failing', dot: 'bg-destructive' },
  stopped: { label: 'Stopped', dot: 'bg-muted-foreground/50' },
}

export function AppsStatusChips({
  counts,
  active,
  onChange,
}: {
  counts: Record<StatusBucket, number>
  active: StatusFilter
  onChange: (next: StatusFilter) => void
}) {
  return (
    <div
      role="group"
      aria-label="Filter by status"
      className="flex flex-wrap items-center gap-2"
    >
      {STATUS_BUCKETS.map((bucket) => {
        const chip = CHIP[bucket]
        const on = active === bucket
        return (
          <button
            key={bucket}
            type="button"
            aria-pressed={on}
            onClick={() => {
              onChange(on ? null : bucket)
            }}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm transition-colors duration-150',
              on
                ? 'border-primary bg-primary/10 text-foreground'
                : 'border-border text-muted-foreground hover:bg-muted/60 hover:text-foreground',
              counts[bucket] === 0 && !on && 'opacity-60',
            )}
          >
            <span
              aria-hidden="true"
              className={cn('size-2 rounded-full', chip.dot)}
            />
            <span className="font-medium tabular-nums text-foreground">
              {counts[bucket]}
            </span>
            {chip.label}
          </button>
        )
      })}
    </div>
  )
}

export function ViewToggle({
  mode,
  onChange,
}: {
  mode: ViewMode
  onChange: (next: ViewMode) => void
}) {
  const btn = (value: ViewMode, label: string, icon: React.ReactNode) => (
    <button
      type="button"
      aria-label={label}
      aria-pressed={mode === value}
      onClick={() => {
        onChange(value)
      }}
      className={cn(
        'rounded-md p-1.5 transition-colors',
        mode === value
          ? 'bg-muted text-foreground'
          : 'text-muted-foreground hover:text-foreground',
      )}
    >
      {icon}
    </button>
  )
  return (
    <div
      role="group"
      aria-label="View mode"
      className="flex items-center gap-0.5 rounded-lg border border-border p-0.5"
    >
      {btn('table', 'Table view', <RowsIcon className="size-4" />)}
      {btn('grid', 'Card view', <GridFourIcon className="size-4" />)}
    </div>
  )
}
