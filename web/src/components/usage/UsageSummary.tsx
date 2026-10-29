import { Display } from '@/components/ui/typography'
import { useTranslation } from 'react-i18next'
import { TrendingDownGlassIcon, TrendingUpGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent } from '@/components/ui/card'
import { formatDeltaPct, formatMoney, groupName } from '../../lib/usageFormat'
import type { UsageReport } from '../../types/usage'

function Stat({
  label,
  value,
  hint,
}: {
  label: string
  value: string
  hint?: string
}) {
  return (
    <Card>
      <CardContent className="space-y-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <Display as="p" size="page" className="tabular-nums">
          {value}
        </Display>
        {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      </CardContent>
    </Card>
  )
}

export function UsageSummary({ report }: { report: UsageReport }) {
  const { t } = useTranslation('usage')
  const money = (v: number) => formatMoney(v, report.currency)
  const change =
    report.totals.delta_pct === undefined
      ? t('summary.noChange')
      : t('summary.change', { value: formatDeltaPct(report.totals.delta_pct) })
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <Stat
        label={t('summary.total')}
        value={money(report.totals.total)}
        hint={change}
      />
      <Stat
        label={t('summary.allocated')}
        value={money(report.totals.allocated)}
      />
      {report.includes_unallocated ? (
        <Stat
          label={t('summary.unallocated')}
          value={money(report.totals.unallocated)}
          hint={t('summary.unallocatedHint')}
        />
      ) : null}
      {report.forecast !== undefined ? (
        <Stat
          label={t('summary.forecast')}
          value={money(report.forecast)}
          hint={t('summary.forecastHint')}
        />
      ) : null}
    </div>
  )
}

export function UsageMovers({ report }: { report: UsageReport }) {
  const { t } = useTranslation('usage')
  const money = (v: number) => formatMoney(Math.abs(v), report.currency)
  return (
    <section className="space-y-2">
      <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
        {t('movers.title')}
      </h2>
      {report.movers.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('movers.none')}</p>
      ) : (
        <ul className="space-y-1.5">
          {report.movers.map((g) => {
            const name = groupName(report, 'resource', g, '')
            const up = g.delta > 0
            return (
              <li key={g.key} className="flex items-center gap-2 text-sm">
                {up ? (
                  <TrendingUpGlassIcon className="size-4.5 text-destructive" />
                ) : (
                  <TrendingDownGlassIcon className="size-4.5 text-muted-foreground" />
                )}
                <span>
                  {t(up ? 'movers.up' : 'movers.down', {
                    name,
                    amount: money(g.delta),
                  })}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
