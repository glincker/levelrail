import '@/styles/material.css'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  ArchiveGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
  planBulk,
  type BulkItemResult as ItemResult,
  type BulkItemStatus as ItemStatus,
  type DbBulkAction,
  type SkipReason,
} from '../lib/databasesFilter'
import { databaseKeys } from '../queries/databases'
import { backupHistoryKeys } from '../queries/backupHistory'
import { postDatabasesBulk } from '../queries/databasesBulk'
import type { DatabaseListEntry } from '../types/databaseDetail'

const STATUS_STYLE: Record<ItemStatus, string> = {
  planned: 'text-foreground',
  ok: 'text-tone-success',
  skipped: 'text-tone-warning',
  denied: 'text-destructive',
  protected: 'text-tone-warning',
  notFound: 'text-destructive',
  error: 'text-destructive',
}

function ResultList({ results }: { results: ItemResult[] }) {
  const { t } = useTranslation('databases')
  return (
    <ul className="max-h-64 divide-y divide-border overflow-auto rounded-md border border-border text-sm">
      {results.map((r) => (
        <li key={r.name} className="flex items-baseline gap-2 px-3 py-1.5">
          <span className="min-w-0 flex-1 truncate font-medium">{r.name}</span>
          <span className={STATUS_STYLE[r.status]}>
            {t(`bulk.result.${r.status}`)}
          </span>
          {r.reason ? (
            <span className="max-w-[45%] truncate text-xs text-muted-foreground">
              {r.status === 'skipped'
                ? t(`bulk.skip.${r.reason as SkipReason}`)
                : r.reason}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

function BulkDialog({
  action,
  databases,
  onClose,
}: {
  action: DbBulkAction
  databases: readonly DatabaseListEntry[]
  onClose: (applied: boolean) => void
}) {
  const { t } = useTranslation('databases')
  const queryClient = useQueryClient()
  const [plan] = useState(() => planBulk(action, databases))
  const [results, setResults] = useState<ItemResult[] | null>(null)
  const [running, setRunning] = useState(false)
  const targets = databases.filter((db) =>
    plan.some((p) => p.name === db.name && p.status === 'planned'),
  )
  const label = t(`bulk.action.${action}`)

  const apply = async () => {
    setRunning(true)
    const names = targets.map((db) => db.name)
    let done: ItemResult[]
    try {
      done = await postDatabasesBulk({
        action,
        names,
        confirm_count: action === 'stop' ? names.length : undefined,
      })
    } catch (err) {
      const reason = err instanceof Error ? err.message : undefined
      done = names.map((name) => ({ name, status: 'error', reason }))
    }
    const byName = new Map(done.map((r) => [r.name, r]))
    setResults(plan.map((p) => byName.get(p.name) ?? p))
    setRunning(false)
    void queryClient.invalidateQueries({ queryKey: databaseKeys.all })
    if (action === 'backup') {
      for (const name of names) {
        void queryClient.invalidateQueries({
          queryKey: backupHistoryKeys.list(name),
        })
      }
    }
  }

  const summary = results
    ? {
        ok: results.filter((r) => r.status === 'ok').length,
        skipped: results.filter((r) => r.status === 'skipped').length,
        failed: results.filter(
          (r) => r.status !== 'ok' && r.status !== 'skipped',
        ).length,
      }
    : null

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !running) onClose(results !== null)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {results
              ? t('bulk.resultsTitle', { action: label })
              : t('bulk.reviewTitle', {
                  action: label,
                  count: databases.length,
                })}
          </DialogTitle>
          <DialogDescription>
            {summary ? t('bulk.summary', summary) : t('bulk.reviewDescription')}
          </DialogDescription>
        </DialogHeader>
        <ResultList results={results ?? plan} />
        {action === 'stop' && !results && targets.length > 0 ? (
          <p className="flex items-start gap-1.5 text-sm text-foreground">
            <TriangleAlertGlassIcon
              className="mt-0.5 size-5 shrink-0 text-tone-warning"
              aria-hidden="true"
            />
            {t('bulk.stopWarning', { count: targets.length })}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={running}
            onClick={() => {
              onClose(results !== null)
            }}
          >
            {results ? t('bulk.close') : t('bulk.cancel')}
          </Button>
          {results ? null : (
            <Button
              type="button"
              variant={action === 'stop' ? 'destructive' : 'default'}
              disabled={targets.length === 0 || running}
              onClick={() => {
                void apply()
              }}
            >
              {running
                ? t('bulk.working')
                : t('bulk.confirm', { action: label, count: targets.length })}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

const ACTIONS: { value: DbBulkAction; icon: typeof PlayGlassIcon }[] = [
  { value: 'start', icon: PlayGlassIcon },
  { value: 'stop', icon: PauseGlassIcon },
  { value: 'backup', icon: ArchiveGlassIcon },
]

/** DatabasesBulkBar runs start, stop or backup on the selected databases. */
export function DatabasesBulkBar({
  selected,
  onClear,
}: {
  selected: readonly DatabaseListEntry[]
  onClear: () => void
}) {
  const { t } = useTranslation('databases')
  const [action, setAction] = useState<DbBulkAction | null>(null)
  const count = selected.length
  return (
    <>
      <p role="status" aria-live="polite" className="sr-only">
        {count > 0 ? t('bulk.selected', { count }) : ''}
      </p>
      {count > 0 ? (
        <div
          role="region"
          aria-label={t('bulk.region')}
          data-dock-avoid=""
          className="fixed bottom-[max(1.5rem,env(safe-area-inset-bottom))] left-1/2 z-30 flex w-max max-w-[calc(100vw-1rem)] -translate-x-1/2 flex-wrap items-center justify-center gap-2 glass glass-thick rounded-xl px-3 py-2"
        >
          <span className="text-sm font-medium" aria-hidden="true">
            {t('bulk.selected', { count })}
          </span>
          {ACTIONS.map(({ value, icon: Icon }) => (
            <Button
              key={value}
              type="button"
              size="sm"
              variant={value === 'stop' ? 'outline' : 'default'}
              onClick={() => {
                setAction(value)
              }}
            >
              <Icon size="1.4em" aria-hidden="true" />
              {t(`bulk.action.${value}`)}
            </Button>
          ))}
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t('bulk.clear')}
            onClick={onClear}
          >
            <XGlassIcon size="1.4em" />
          </Button>
        </div>
      ) : null}
      {action ? (
        <BulkDialog
          action={action}
          databases={selected}
          onClose={(applied) => {
            setAction(null)
            if (applied) onClear()
          }}
        />
      ) : null}
    </>
  )
}
