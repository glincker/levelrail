import { GridFourIcon, RowsIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { TONE } from '@/components/kit/tone'
import {
  STATUS_BUCKETS,
  type StatusBucket,
  type StatusFilter,
  type ViewMode,
} from '../../lib/appsListView'

const CHIP: Record<StatusBucket, { label: string; dot: string }> = {
  running: { label: 'Running', dot: TONE.success.solid },
  deploying: { label: 'Deploying', dot: TONE.warning.solid },
  failing: { label: 'Failing', dot: TONE.danger.solid },
  stopped: { label: 'Stopped', dot: TONE.neutral.solid },
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
      className="-mx-4 flex items-center gap-2 overflow-x-auto px-4 pb-1 sm:mx-0 sm:flex-wrap sm:overflow-visible sm:px-0 sm:pb-0"
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
              'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1 text-sm transition-colors duration-150',
              on
                ? 'border-ring bg-tone-accent-soft text-foreground'
                : 'border-border text-muted-foreground hover:bg-muted/60 hover:text-foreground',
              counts[bucket] === 0 && !on && 'text-muted-foreground/70',
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
