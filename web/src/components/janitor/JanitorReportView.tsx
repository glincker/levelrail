import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { formatSize } from '../../lib/format'
import type { JanitorItem, JanitorReport } from '../../queries/janitor'

const MAX_ITEMS = 50

function actionVariant(action: JanitorItem['action']) {
  switch (action) {
    case 'removed':
      return 'success'
    case 'would_remove':
    case 'would_quarantine':
      return 'warning'
    case 'quarantined':
      return 'info'
    default:
      return 'muted'
  }
}

export function JanitorReportView({ report }: { report: JanitorReport }) {
  const { t } = useTranslation('janitor')
  return (
    <div className="space-y-4">
      {report.stages.map((s) => {
        const shown = s.items.slice(0, MAX_ITEMS)
        return (
          <section key={s.stage} aria-label={t(`stage.${s.stage}`)}>
            <h3 className="text-sm font-medium text-foreground">
              {t(`stage.${s.stage}`)}
            </h3>
            {s.status === 'skipped' ? (
              <p className="text-sm text-muted-foreground">
                {t('report.skipped', { reason: s.reason })}
              </p>
            ) : null}
            {s.error ? (
              <p role="alert" className="text-sm text-destructive">
                {t('report.failed', { error: s.error })}
              </p>
            ) : null}
            {s.status === 'ran' && s.items.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('report.empty')}
              </p>
            ) : null}
            {shown.length > 0 ? (
              <ul className="divide-y divide-border">
                {shown.map((it) => (
                  <li
                    key={`${it.action}:${it.name}`}
                    className="flex flex-wrap items-center gap-2 py-1.5 text-sm"
                  >
                    <Badge variant={actionVariant(it.action)}>
                      {t(`action.${it.action}`)}
                    </Badge>
                    <span className="font-medium text-foreground">
                      {it.name}
                    </span>
                    {it.detail ? (
                      <span className="text-muted-foreground">{it.detail}</span>
                    ) : null}
                    <span className="ml-auto text-muted-foreground">
                      {it.size_bytes > 0 ? formatSize(it.size_bytes) : ''}
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
            {s.items.length > MAX_ITEMS ? (
              <p className="text-xs text-muted-foreground">
                {t('report.more', { count: s.items.length - MAX_ITEMS })}
              </p>
            ) : null}
          </section>
        )
      })}
      {report.dry_run ? (
        <p className="text-sm text-muted-foreground">
          {t('report.previewNote')}
        </p>
      ) : null}
    </div>
  )
}
