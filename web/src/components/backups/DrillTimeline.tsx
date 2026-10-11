import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { formatDate } from '../../lib/format'
import { useBackupDrills } from '../../queries/backupProtection'
import type { BackupDrill } from '../../types/backupProtection'

const COLLAPSED_COUNT = 4

function resourceName(d: BackupDrill): string {
  return d.resource_kind === 'volume'
    ? `${d.service_name ?? ''}/${d.volume_name ?? ''}`
    : (d.database_name ?? '')
}

const DOT: Record<BackupDrill['status'], string> = {
  passed: 'bg-tone-success-solid',
  failed: 'bg-tone-danger-solid',
  running: 'bg-tone-info-solid',
}

/** DrillTimeline is the small restore-drill history card: the latest drills, collapsed by default. */
export function DrillTimeline() {
  const { t } = useTranslation('backups')
  const drills = useBackupDrills({ limit: 25 })
  const [expanded, setExpanded] = useState(false)
  const all = drills.data ?? []
  const shown = expanded ? all : all.slice(0, COLLAPSED_COUNT)

  return (
    <section
      className="space-y-3 rounded-lg border border-border p-4"
      aria-labelledby="drill-timeline-title"
    >
      <div>
        <h2 id="drill-timeline-title" className="text-base font-medium">
          {t('drills.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('drills.shortDescription')}
        </p>
      </div>
      {drills.isError ? (
        <p className="text-sm text-destructive">
          {t('drills.loadFailed', { error: drills.error.message })}
        </p>
      ) : all.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('drills.empty')}</p>
      ) : (
        <>
          <ol className="space-y-2">
            {shown.map((d) => (
              <li key={d.id} className="flex items-start gap-2 text-sm">
                <span
                  aria-hidden="true"
                  className={cn(
                    'mt-1.5 size-2 shrink-0 rounded-full',
                    DOT[d.status],
                  )}
                />
                <div className="min-w-0">
                  <p className="truncate">
                    <span className="font-medium">{resourceName(d)}</span>{' '}
                    <span className="text-muted-foreground">
                      {t(`drills.status.${d.status}`)}
                    </span>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {formatDate(d.started_at, '-')}
                    {' · '}
                    {t(`drills.trigger.${d.trigger}`)}
                    {d.status === 'failed' && d.error
                      ? ` · ${t('drills.stage', { stage: d.stage })}: ${d.error}`
                      : ''}
                  </p>
                </div>
              </li>
            ))}
          </ol>
          {all.length > COLLAPSED_COUNT ? (
            <button
              type="button"
              className="text-sm text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
              onClick={() => {
                setExpanded((v) => !v)
              }}
            >
              {expanded
                ? t('drills.showLess')
                : t('drills.showAll', { count: all.length })}
            </button>
          ) : null}
        </>
      )}
    </section>
  )
}
