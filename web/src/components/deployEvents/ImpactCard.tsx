import { useTranslation } from 'react-i18next'
import { formatMs, formatPercent } from '../../lib/deployEvents'
import type {
  DeployEventImpact,
  DeployEventWindow,
} from '../../types/deployEvents'

function Side({
  label,
  window: w,
}: {
  label: string
  window: DeployEventWindow
}) {
  const { t } = useTranslation('deployEvents')
  return (
    <div className="min-w-0 flex-1">
      <h4 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </h4>
      {w.has_traffic ? (
        <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-sm">
          <dt className="text-muted-foreground">{t('impact.rate')}</dt>
          <dd className="tabular-nums">{w.rate_per_sec.toFixed(2)}</dd>
          <dt className="text-muted-foreground">{t('impact.errors')}</dt>
          <dd className="tabular-nums">{formatPercent(w.error_rate_5xx)}</dd>
          <dt className="text-muted-foreground">{t('impact.p95')}</dt>
          <dd className="tabular-nums">{formatMs(w.p95_ms)}</dd>
        </dl>
      ) : (
        <p className="mt-1 text-sm text-muted-foreground">
          {t('impact.noTraffic')}
        </p>
      )}
    </div>
  )
}

export function ImpactCard({ impact }: { impact: DeployEventImpact }) {
  const { t } = useTranslation('deployEvents')
  return (
    <section
      aria-label={t('impact.title')}
      className="mt-2 rounded-lg border bg-muted/30 p-3"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:gap-6">
        <Side label={t('impact.before')} window={impact.before} />
        <Side label={t('impact.after')} window={impact.after} />
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        {impact.complete
          ? t('impact.window', { window: formatMs(impact.window_ms) })
          : t('impact.open')}
      </p>
    </section>
  )
}
