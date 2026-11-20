import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type { AutoStatus, AutoStatusState } from '@/queries/upgradeSchedule'

const TONES = {
  off: 'muted',
  paused: 'info',
  blocked: 'destructive',
  degraded: 'warning',
  ready: 'success',
} as const satisfies Record<AutoStatusState, string>

// The command the upgrade docs give for applying an update from the host.
const MANUAL_COMMAND = 'levelrail-cli upgrade --apply'

function when(iso: string | undefined): string | null {
  if (!iso) return null
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? null : d.toLocaleString()
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-wrap gap-x-1.5">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{value}</dd>
    </div>
  )
}

/** AutoUpdateStatus says whether automatic updates work on this host and, if not, why. */
export function AutoUpdateStatus({ status }: { status: AutoStatus }) {
  const { t } = useTranslation('updates')
  const reason = status.reason || status.last_check_error || ''
  const lastCheck = when(status.last_check_at)
  const nextTry = when(status.next_check_at)
  const resultAt = when(status.last_result_at)
  const result = status.last_result
    ? t(`autoStatus.result.${status.last_result}`, {
        defaultValue: status.last_result,
      })
    : null

  const listAgeHours =
    status.last_good_list_at && status.list_age_seconds !== undefined
      ? Math.floor(status.list_age_seconds / 3600)
      : null
  // Only worth a row once the list is stale or came from the fallback source.
  const showList =
    listAgeHours !== null &&
    (status.check_failures > 0 || status.list_source === 'fallback')
  const listValue = showList
    ? [
        t('autoStatus.listAge', { hours: listAgeHours }),
        status.list_source === 'fallback'
          ? t('autoStatus.listSourceFallback')
          : null,
      ]
        .filter(Boolean)
        .join(', ')
    : ''

  return (
    <div className="space-y-2 border-t border-border pt-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={TONES[status.state]}>
          {t(`autoStatus.state.${status.state}`)}
        </Badge>
        <p role="status" className="min-w-0 flex-1 text-sm">
          {t(`autoStatus.summary.${status.state}`, { reason })}
        </p>
      </div>
      {!status.can_apply ? (
        <div className="flex items-start gap-2 rounded-md border border-tone-warning-border bg-tone-warning-soft p-2 text-sm">
          <TriangleAlertGlassIcon
            aria-hidden="true"
            className="mt-0.5 size-4 shrink-0 text-tone-warning"
          />
          <div className="min-w-0 space-y-1">
            <p>{t('autoStatus.manual.intro')}</p>
            <code className="block overflow-x-auto rounded bg-muted px-2 py-1 font-mono text-xs">
              {MANUAL_COMMAND}
            </code>
            <p className="text-xs text-muted-foreground">
              {t('autoStatus.manual.docker')}
            </p>
          </div>
        </div>
      ) : null}
      {status.signature_pinned === false ? (
        <p className="text-xs text-muted-foreground">
          {t('autoStatus.unpinned')}
        </p>
      ) : null}
      <dl className="grid gap-x-6 gap-y-1 text-xs sm:grid-cols-2">
        {lastCheck ? (
          <Row label={t('autoStatus.lastCheck')} value={lastCheck} />
        ) : null}
        {status.check_failures > 0 ? (
          <Row
            label={t('autoStatus.failures')}
            value={String(status.check_failures)}
          />
        ) : null}
        {showList ? (
          <Row label={t('autoStatus.lastGoodList')} value={listValue} />
        ) : null}
        {nextTry ? (
          <Row label={t('autoStatus.nextTry')} value={nextTry} />
        ) : null}
        {result ? (
          <Row
            label={t('autoStatus.lastResult')}
            value={[result, resultAt, status.last_result_reason]
              .filter(Boolean)
              .join(', ')}
          />
        ) : null}
      </dl>
    </div>
  )
}
