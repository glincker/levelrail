import { useId } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'

// One shared row for per-domain settings: the current state is always
// visible (status, key numbers), and the editor opens only on an explicit
// Edit button, so nothing important hides behind a disclosure.
export function SettingSummary({
  title,
  status,
  summary,
  note,
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
  actions?: ReactNode
  editLabel: string
  closeLabel: string
  editing: boolean
  onEditingChange: (editing: boolean) => void
  children?: ReactNode
}) {
  const panelId = useId()
  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
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
            variant="ghost"
            size="sm"
            aria-expanded={editing}
            aria-controls={panelId}
            onClick={() => onEditingChange(!editing)}
          >
            {editing ? closeLabel : editLabel}
          </Button>
        </div>
      </div>
      {note ? <div className="mt-1">{note}</div> : null}
      <div id={panelId}>{editing ? children : null}</div>
    </div>
  )
}
