import { useId } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useCompactSettings } from './compactSettings'

// One shared row for per-domain settings: the current state is always
// visible (status, key numbers), and the editor opens only on an explicit
// Edit button, so nothing important hides behind a disclosure.
export function SettingSummary({
  title,
  status,
  summary,
  note,
  help,
  actions,
  editLabel,
  closeLabel,
  editing,
  onEditingChange,
  children,
}: {
  title?: ReactNode
  status: ReactNode
  summary?: ReactNode
  note?: ReactNode
  help?: string
  actions?: ReactNode
  editLabel: string
  closeLabel: string
  editing: boolean
  onEditingChange: (editing: boolean) => void
  children?: ReactNode
}) {
  const panelId = useId()
  const compact = useCompactSettings()
  return (
    <div
      title={compact ? help : undefined}
      className={cn(
        'text-sm',
        compact ? 'py-1.5' : 'rounded-md border border-border bg-muted/30 p-3',
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div
          className={cn(
            'flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1',
            // Flatten badges to plain labels so an empty state is one muted line.
            compact &&
              '[&_[data-slot=badge]]:border-0 [&_[data-slot=badge]]:bg-transparent [&_[data-slot=badge]]:px-0 [&_[data-slot=badge]]:py-0',
          )}
        >
          {title ? (
            <span className="font-medium text-foreground">{title}</span>
          ) : null}
          {status}
          {summary ? (
            <span className="text-xs text-muted-foreground">{summary}</span>
          ) : null}
        </div>
        <div className="flex items-center gap-1">
          {actions}
          <Button
            type="button"
            variant={compact ? 'link' : 'ghost'}
            size="sm"
            className={compact ? 'h-6 px-1' : undefined}
            aria-expanded={editing}
            aria-controls={panelId}
            onClick={() => onEditingChange(!editing)}
          >
            {editing ? closeLabel : editLabel}
          </Button>
        </div>
      </div>
      {note ? <div className={compact ? '' : 'mt-1'}>{note}</div> : null}
      <div id={panelId}>{editing ? children : null}</div>
    </div>
  )
}
