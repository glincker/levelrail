import { useEffect, useRef, useState } from 'react'
import { WarningIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  useBulkApps,
  type BulkAction,
  type BulkAppResult,
  type BulkResponse,
} from '../queries/appsBulk'

const ACTIONS: { value: BulkAction; label: string; needsValue?: string }[] = [
  { value: 'redeploy', label: 'Redeploy' },
  { value: 'restart', label: 'Restart' },
  { value: 'stop', label: 'Stop' },
  { value: 'start', label: 'Start' },
  { value: 'add-tag', label: 'Add tag', needsValue: 'Tag (key or key:value)' },
  { value: 'remove-tag', label: 'Remove tag', needsValue: 'Tag to remove' },
  {
    value: 'set-environment',
    label: 'Set environment',
    needsValue: 'Environment name or ID',
  },
  {
    value: 'move-to-project',
    label: 'Move to project',
    needsValue: 'Project ID',
  },
  { value: 'delete', label: 'Delete' },
]

const STATUS_STYLE: Record<BulkAppResult['status'], string> = {
  ok: 'text-emerald-600 dark:text-emerald-400',
  would_apply: 'text-foreground',
  denied: 'text-destructive',
  not_found: 'text-destructive',
  skipped: 'text-amber-600 dark:text-amber-400',
  error: 'text-destructive',
}

function ResultList({ results }: { results: BulkAppResult[] }) {
  return (
    <ul className="max-h-64 divide-y divide-border overflow-auto rounded-md border border-border text-sm">
      {results.map((r) => (
        <li key={r.name} className="flex items-baseline gap-2 px-3 py-1.5">
          <span className="min-w-0 flex-1 truncate font-medium">{r.name}</span>
          <span className={STATUS_STYLE[r.status]}>
            {r.status.replace('_', ' ')}
          </span>
          {r.message ? (
            <span className="max-w-[45%] truncate text-xs text-muted-foreground">
              {r.message}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

function BulkDialog({
  action,
  value,
  names,
  onClose,
}: {
  action: BulkAction
  value: string
  names: string[]
  onClose: (applied: boolean) => void
}) {
  const bulk = useBulkApps()
  const [plan, setPlan] = useState<BulkResponse | null>(null)
  const [applied, setApplied] = useState<BulkResponse | null>(null)
  const started = useRef(false)

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    bulk.mutate(
      { action, names, value, dry_run: true },
      {
        onSuccess: (data) => {
          setPlan(data)
        },
      },
    )
  }, [action, names, value, bulk])

  const targets = (plan?.results ?? [])
    .filter((r) => r.status === 'would_apply')
    .map((r) => r.name)
  const label = ACTIONS.find((a) => a.value === action)?.label ?? action

  const apply = () => {
    bulk.mutate(
      {
        action,
        names: targets,
        value,
        confirm_names: action === 'delete' ? targets : undefined,
      },
      {
        onSuccess: (data) => {
          setApplied(data)
        },
      },
    )
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          onClose(applied !== null)
        }
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {applied ? `${label}: results` : `${label} ${names.length} app(s)`}
          </DialogTitle>
          <DialogDescription>
            {applied
              ? 'Each app was checked against your permissions on its own.'
              : 'Apps you may not act on are listed as denied and left alone.'}
          </DialogDescription>
        </DialogHeader>
        {applied ? (
          <ResultList results={applied.results} />
        ) : plan ? (
          <>
            <ResultList results={plan.results} />
            {action === 'delete' && targets.length > 0 ? (
              <p className="flex items-start gap-1.5 text-sm text-destructive">
                <WarningIcon
                  className="mt-0.5 size-4 shrink-0"
                  aria-hidden="true"
                />
                This permanently deletes {targets.length} app(s). It is recorded
                in the audit log for each app.
              </p>
            ) : null}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">Checking targets...</p>
        )}
        {bulk.isError ? (
          <p className="text-sm text-destructive">{bulk.error.message}</p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              onClose(applied !== null)
            }}
          >
            {applied ? 'Close' : 'Cancel'}
          </Button>
          {applied ? null : (
            <Button
              type="button"
              variant={action === 'delete' ? 'destructive' : 'default'}
              disabled={targets.length === 0 || bulk.isPending}
              onClick={apply}
            >
              {bulk.isPending ? 'Working...' : `${label} ${targets.length}`}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function AppsBulkBar({
  selected,
  onClear,
}: {
  selected: string[]
  onClear: () => void
}) {
  const [action, setAction] = useState<BulkAction>('restart')
  const [value, setValue] = useState('')
  const [open, setOpen] = useState(false)
  const spec = ACTIONS.find((a) => a.value === action)

  if (selected.length === 0) {
    return null
  }
  const ready = !spec?.needsValue || value.trim() !== ''

  return (
    <>
      <div
        role="region"
        aria-label="Bulk actions"
        className="fixed bottom-6 left-1/2 z-30 flex -translate-x-1/2 items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 shadow-lg"
      >
        <span className="text-sm font-medium">{selected.length} selected</span>
        <select
          aria-label="Bulk action"
          value={action}
          className="h-8 rounded-md border border-input bg-background px-2 text-sm"
          onChange={(e) => {
            setAction(e.target.value as BulkAction)
            setValue('')
          }}
        >
          {ACTIONS.map((a) => (
            <option key={a.value} value={a.value}>
              {a.label}
            </option>
          ))}
        </select>
        {spec?.needsValue ? (
          <Input
            value={value}
            placeholder={spec.needsValue}
            aria-label={spec.needsValue}
            className="h-8 w-48"
            onChange={(e) => {
              setValue(e.target.value)
            }}
          />
        ) : null}
        <Button
          type="button"
          size="sm"
          variant={action === 'delete' ? 'destructive' : 'default'}
          disabled={!ready}
          onClick={() => {
            setOpen(true)
          }}
        >
          Review
        </Button>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label="Clear selection"
          onClick={onClear}
        >
          <XIcon />
        </Button>
      </div>
      {open ? (
        <BulkDialog
          action={action}
          value={value.trim()}
          names={selected}
          onClose={(applied) => {
            setOpen(false)
            if (applied) {
              onClear()
            }
          }}
        />
      ) : null}
    </>
  )
}
