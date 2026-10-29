import { useTranslation } from 'react-i18next'
import { TrendingUpGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDeltaPct, formatMoney } from '../../lib/usageFormat'
import type { CostExploration } from '../../types/costExplorer'
import { seriesLabel } from './costLabels'

const GRID =
  'grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 sm:grid-cols-[minmax(0,1fr)_7rem_7rem_5rem]'

// Ranked cost per group with the change against the previous period.
export function CostBreakdown({ data }: { data: CostExploration }) {
  const { t } = useTranslation('costs')
  if (data.series.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('empty.noData')}</p>
  }
  return (
    <div role="table" aria-label={t('table.label')} className="text-sm">
      <div
        role="row"
        className={`${GRID} border-b border-border pb-2 text-xs text-muted-foreground`}
      >
        <span role="columnheader">{t('table.name')}</span>
        <span role="columnheader" className="text-right">
          {t('table.cost')}
        </span>
        <span role="columnheader" className="hidden text-right sm:block">
          {t('table.previous')}
        </span>
        <span role="columnheader" className="text-right">
          {t('table.change')}
        </span>
      </div>
      {data.series.map((s) => (
        <div
          key={s.key}
          role="row"
          className={`${GRID} border-b border-border/50 py-2 last:border-0`}
        >
          <span role="cell" className="truncate">
            {seriesLabel(data, s, t)}
          </span>
          <span role="cell" className="text-right tabular-nums">
            {formatMoney(s.total, data.currency)}
          </span>
          <span
            role="cell"
            className="hidden text-right tabular-nums text-muted-foreground sm:block"
          >
            {formatMoney(s.previous, data.currency)}
          </span>
          <span role="cell" className="text-right tabular-nums">
            {formatDeltaPct(s.delta_pct)}
          </span>
        </div>
      ))}
    </div>
  )
}

// The groups whose cost grew most against the previous period.
export function CostMovers({ data }: { data: CostExploration }) {
  const { t } = useTranslation('costs')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('movers.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('movers.description')}
        </p>
      </CardHeader>
      <CardContent>
        {data.movers.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('movers.none')}</p>
        ) : (
          <ul className="space-y-1.5">
            {data.movers.map((m) => (
              <li key={m.key} className="flex items-center gap-2 text-sm">
                <TrendingUpGlassIcon className="size-4.5 shrink-0 text-destructive" />
                <span className="min-w-0 truncate">
                  {t('movers.up', {
                    name: seriesLabel(data, m, t),
                    amount: formatMoney(m.delta, data.currency),
                    pct: formatDeltaPct(m.delta_pct),
                  })}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
