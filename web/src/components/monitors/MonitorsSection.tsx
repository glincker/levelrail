import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import {
  deriveMonitorState,
  lastCheckAt,
  monitorTarget,
  uptimePercent,
  type MonitorState,
} from '@/lib/monitors'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import {
  useDeleteMonitor,
  useMonitorResults,
  useMonitors,
} from '../../queries/monitors'
import type { Monitor } from '../../types/monitor'
import { MonitorForm } from './MonitorForm'

const STATE_VARIANT: Record<
  MonitorState,
  'success' | 'destructive' | 'muted' | 'warning'
> = {
  up: 'success',
  suspect: 'warning',
  down: 'destructive',
  recovering: 'warning',
  degraded: 'warning',
  paused: 'muted',
  unknown: 'muted',
}

export function MonitorRow({
  monitor,
  onDelete,
}: {
  monitor: Monitor
  onDelete: (m: Monitor) => void
}) {
  const { t } = useTranslation('monitors')
  const { data } = useMonitorResults(monitor.id)
  const state = deriveMonitorState(monitor, data)
  const last = lastCheckAt(data)
  const lastIso = last === undefined ? undefined : new Date(last).toISOString()
  const uptime = uptimePercent(data)
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2 text-sm">
      <span className="min-w-32 flex-1 font-medium break-words">
        {monitor.name}
      </span>
      <span className="min-w-40 flex-[2] font-mono text-xs break-all text-muted-foreground">
        {monitorTarget(monitor)}
      </span>
      <Badge variant={STATE_VARIANT[state]}>{t(`state.${state}`)}</Badge>
      <span
        className="w-28 text-xs text-muted-foreground"
        title={lastIso ? formatAbsoluteTime(lastIso) : undefined}
      >
        {lastIso ? formatRelativeTime(lastIso) : t('never')}
      </span>
      <span className="w-14 text-right text-xs tabular-nums">
        {uptime === undefined ? '-' : `${uptime.toFixed(1)}%`}
      </span>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('delete.action', { name: monitor.name })}
        onClick={() => {
          onDelete(monitor)
        }}
      >
        <TrashGlassIcon className="size-5" aria-hidden="true" />
      </Button>
    </li>
  )
}

export function MonitorsList({
  monitors,
  onDelete,
}: {
  monitors: Monitor[]
  onDelete: (m: Monitor) => void
}) {
  const { t } = useTranslation('monitors')
  if (monitors.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('empty')}</p>
  }
  return (
    <ul className="divide-y divide-border rounded-lg border border-border bg-card">
      {monitors.map((m) => (
        <MonitorRow key={m.id} monitor={m} onDelete={onDelete} />
      ))}
    </ul>
  )
}

// Hidden entirely when the API answers 404/501 (monitors off).
export function MonitorsSection() {
  const { t } = useTranslation('monitors')
  const query = useMonitors()
  const del = useDeleteMonitor()
  const [adding, setAdding] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<Monitor | null>(null)

  if (!query.isError && !query.data) return null

  return (
    <section aria-labelledby="monitors-title" className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h2 id="monitors-title" className="text-sm font-medium">
          {t('title')}
        </h2>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            setAdding((v) => !v)
          }}
        >
          <PlusGlassIcon className="size-5" aria-hidden="true" />
          {adding ? t('cancel') : t('add')}
        </Button>
      </div>
      {adding ? (
        <MonitorForm
          onDone={() => {
            setAdding(false)
          }}
        />
      ) : null}
      {query.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('loadError', { message: query.error.message })}
        </p>
      ) : (
        <MonitorsList monitors={query.data ?? []} onDelete={setPendingDelete} />
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
        title={t('delete.title', { name: pendingDelete?.name ?? '' })}
        description={t('delete.description')}
        confirmLabel={t('delete.confirm')}
        pendingLabel={t('delete.pending')}
        tone="destructive"
        pending={del.isPending}
        error={del.error?.message ?? null}
        onConfirm={() => {
          if (!pendingDelete) return
          const { id, name } = pendingDelete
          del.mutate(id, {
            onSuccess: () => {
              toast.add({ title: t('delete.done', { name }), type: 'success' })
              setPendingDelete(null)
            },
          })
        }}
      />
    </section>
  )
}
