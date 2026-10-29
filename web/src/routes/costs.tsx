import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CoinsGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { CostSectionTabs } from '@/components/shell/SectionTabGroups'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { CostBreakdown, CostMovers } from '../components/costs/CostBreakdown'
import { CostBudgetsPanel } from '../components/costs/CostBudgetsPanel'
import { CostForecastCard } from '../components/costs/CostForecastCard'
import { CostTrendChart } from '../components/costs/CostTrendChart'
import { useCostExplore } from '../queries/costExplorer'
import { formatDeltaPct, formatMoney } from '../lib/usageFormat'
import type {
  CostExploreParams,
  CostGranularity,
  CostGroupBy,
  CostPeriod,
} from '../types/costExplorer'

export const Route = createFileRoute('/costs')({
  component: CostsPage,
  pendingComponent: CostsSkeleton,
})

const GROUPS: CostGroupBy[] = ['app', 'project', 'tag', 'node', 'kind']
const PERIODS: CostPeriod[] = ['mtd', '30d', 'last_month']
const MONTH_CHOICES = [3, 6, 12]
const FORECAST_PARAMS: CostExploreParams = {
  groupBy: 'kind',
  granularity: 'daily',
  period: 'mtd',
  months: 6,
}

function CostsSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <Skeleton className="h-6 w-48" />
      <Skeleton className="h-24 w-full rounded-lg" />
      <Skeleton className="h-64 w-full rounded-lg" />
    </div>
  )
}

function ToggleGroup<T extends string | number>({
  label,
  options,
  value,
  onChange,
  render,
}: {
  label: string
  options: T[]
  value: T
  onChange: (v: T) => void
  render: (v: T) => string
}) {
  return (
    <div className="flex flex-wrap gap-1" role="group" aria-label={label}>
      {options.map((o) => (
        <Button
          key={o}
          size="sm"
          variant={value === o ? 'secondary' : 'ghost'}
          aria-pressed={value === o}
          onClick={() => onChange(o)}
        >
          {render(o)}
        </Button>
      ))}
    </div>
  )
}

function CostsPage() {
  const { t } = useTranslation('costs')
  const [groupBy, setGroupBy] = useState<CostGroupBy>('app')
  const [granularity, setGranularity] = useState<CostGranularity>('daily')
  const [period, setPeriod] = useState<CostPeriod>('mtd')
  const [months, setMonths] = useState(6)
  const explore = useCostExplore({ groupBy, granularity, period, months })
  const forecast = useCostExplore(FORECAST_PARAMS)
  const data = explore.data

  return (
    <PageTemplate
      title={t('title')}
      description={t('description')}
      helpPath="/cost-explorer"
      helpLabel={t('title')}
      tabs={<CostSectionTabs />}
    >
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
        <ToggleGroup
          label={t('groupBy.label')}
          options={GROUPS}
          value={groupBy}
          onChange={setGroupBy}
          render={(g) => t(`groupBy.${g}`)}
        />
        <ToggleGroup
          label={t('granularity.label')}
          options={['daily', 'monthly'] as CostGranularity[]}
          value={granularity}
          onChange={setGranularity}
          render={(g) => t(`granularity.${g}`)}
        />
        {granularity === 'daily' ? (
          <ToggleGroup
            label={t('period.label')}
            options={PERIODS}
            value={period}
            onChange={setPeriod}
            render={(p) => t(`period.${p}`)}
          />
        ) : (
          <ToggleGroup
            label={t('months.label')}
            options={MONTH_CHOICES}
            value={months}
            onChange={setMonths}
            render={(m) => t('months.count', { count: m })}
          />
        )}
      </div>
      {explore.isPending ? (
        <CostsSkeleton />
      ) : explore.isError ? (
        <p className="text-sm text-destructive" role="alert">
          {t('error', { message: explore.error.message })}
        </p>
      ) : data ? (
        <>
          {!data.telemetry_available ? (
            <EmptyState
              icon={<CoinsGlassIcon size="1.4em" />}
              title={t('empty.noTelemetryTitle')}
              description={t('empty.noTelemetryDescription')}
            />
          ) : !data.configured ? (
            <EmptyState
              icon={<CoinsGlassIcon size="1.4em" />}
              title={t('empty.notConfiguredTitle')}
              description={t('empty.notConfiguredDescription')}
              action={
                <Button render={<a href="/usage" />}>
                  {t('empty.configure')}
                </Button>
              }
            />
          ) : null}
          <Card>
            <CardContent className="flex flex-wrap items-baseline gap-x-4 gap-y-1 pt-4">
              <p className="text-2xl font-semibold tabular-nums">
                {formatMoney(data.total, data.currency)}
              </p>
              <p className="text-sm text-muted-foreground">
                {data.delta_pct === undefined
                  ? t('summary.noChange')
                  : t('summary.change', {
                      value: formatDeltaPct(data.delta_pct),
                    })}
              </p>
            </CardContent>
          </Card>
          <CostTrendChart data={data} />
          <div className="grid gap-4 lg:grid-cols-2">
            {forecast.data ? <CostForecastCard data={forecast.data} /> : null}
            <CostMovers data={data} />
          </div>
          <CostBreakdown data={data} />
        </>
      ) : null}
      <CostBudgetsPanel />
    </PageTemplate>
  )
}
