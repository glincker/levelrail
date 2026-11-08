import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBuildAction, type BuildRow } from '../../queries/buildCenter'

interface Props {
  rows: BuildRow[]
  onClear: () => void
}

export function BuildsBulkBar({ rows, onClear }: Props) {
  const { t } = useTranslation('buildCenter')
  const cancel = useBuildAction('cancel')
  const retry = useBuildAction('retry')
  const [busy, setBusy] = useState(false)
  const cancelable = rows.filter((r) => r.can_cancel)
  const retryable = rows.filter((r) => r.can_retry)

  // One request per row so a single failure never blocks the rest.
  const run = async (kind: 'cancel' | 'retry', targets: BuildRow[]) => {
    const m = kind === 'cancel' ? cancel : retry
    setBusy(true)
    const results = await Promise.allSettled(
      targets.map((r) => m.mutateAsync(r)),
    )
    setBusy(false)
    const failed = results.filter((r) => r.status === 'rejected').length
    const ok = results.length - failed
    toast.add({
      title: t(`bulk.${kind}Result`, { ok, failed }),
      type: failed > 0 ? 'error' : 'success',
    })
    if (failed === 0) onClear()
  }

  return (
    <div
      className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-sm"
      role="region"
      aria-label={t('bulk.label')}
    >
      <span className="font-medium">
        {t('bulk.selected', { count: rows.length })}
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={busy || cancelable.length === 0}
        onClick={() => {
          void run('cancel', cancelable)
        }}
      >
        {t('bulk.cancel', { count: cancelable.length })}
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={busy || retryable.length === 0}
        onClick={() => {
          void run('retry', retryable)
        }}
      >
        {t('bulk.retry', { count: retryable.length })}
      </Button>
      <Button variant="ghost" size="sm" disabled={busy} onClick={onClear}>
        {t('bulk.clear')}
      </Button>
    </div>
  )
}
