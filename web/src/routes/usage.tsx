import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CoinsGlassIcon, DownloadGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { CostSectionTabs } from '@/components/shell/SectionTabGroups'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { BudgetsPanel } from '../components/usage/BudgetsPanel'
import { UsageChart } from '../components/usage/UsageChart'
import { UsageSettingsDialog } from '../components/usage/UsageSettingsDialog'
import { UsageMovers, UsageSummary } from '../components/usage/UsageSummary'
import { UsageTable } from '../components/usage/UsageTable'
import { ApiError } from '../lib/apiError'
import { useUsageReport, useUsageSettings } from '../queries/usage'
import type { UsagePeriod } from '../types/usage'

export const Route = createFileRoute('/usage')({
  component: UsagePage,
  pendingComponent: UsageSkeleton,
})

const PERIODS: UsagePeriod[] = ['mtd', '30d', 'last_month']

function UsageSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <Skeleton className="h-6 w-48" />
      <Skeleton className="h-24 w-full rounded-lg" />
      <Skeleton className="h-56 w-full rounded-lg" />
    </div>
  )
}

function UsagePage() {
  const { t } = useTranslation('usage')
  const [period, setPeriod] = useState<UsagePeriod>('mtd')
  const [pricesOpen, setPricesOpen] = useState(false)
  const report = useUsageReport(period)
  const settings = useUsageSettings()
  const canPrice = settings.isSuccess
  const settingsForbidden =
    settings.error instanceof ApiError && settings.error.status === 403
  const data = report.data

  return (
    <PageTemplate
      title={t('title')}
      description={t('description')}
      helpPath="/usage-and-cost"
      helpLabel={t('title')}
      tabs={<CostSectionTabs />}
      actions={
        <>
          {canPrice ? (
            <Button variant="outline" onClick={() => setPricesOpen(true)}>
              {t('settings.open')}
            </Button>
          ) : null}
          <Button
            variant="outline"
            render={
              <a
                href={`/api/v1/usage/report?period=${period}&format=csv`}
                download
              />
            }
          >
            <DownloadGlassIcon size="1.2em" />
            {t('export.csv')}
          </Button>
        </>
      }
    >
      <div
        className="flex flex-wrap gap-1"
        role="group"
        aria-label={t('period.label')}
      >
        {PERIODS.map((p) => (
          <Button
            key={p}
            size="sm"
            variant={period === p ? 'secondary' : 'ghost'}
            aria-pressed={period === p}
            onClick={() => setPeriod(p)}
          >
            {t(`period.${p}`)}
          </Button>
        ))}
      </div>
      {report.isPending ? (
        <UsageSkeleton />
      ) : report.isError ? (
        <p className="text-sm text-destructive" role="alert">
          {t('error', { message: report.error.message })}
        </p>
      ) : data ? (
        <>
          {!data.telemetry_available ? (
            <EmptyState
              icon={<CoinsGlassIcon size="1.2em" />}
              title={t('empty.noTelemetryTitle')}
              description={t('empty.noTelemetryDescription')}
            />
          ) : !data.configured ? (
            <EmptyState
              icon={<CoinsGlassIcon size="1.2em" />}
              title={t('empty.notConfiguredTitle')}
              description={
                settingsForbidden || !canPrice
                  ? t('empty.notConfiguredAskAdmin')
                  : t('empty.notConfiguredDescription')
              }
              action={
                canPrice ? (
                  <Button onClick={() => setPricesOpen(true)}>
                    {t('empty.configure')}
                  </Button>
                ) : undefined
              }
            />
          ) : null}
          {data.unpriced_nodes.length > 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('unpriced', { nodes: data.unpriced_nodes.join(', ') })}
            </p>
          ) : null}
          <UsageSummary report={data} />
          <UsageChart report={data} />
          <UsageTable report={data} />
          <UsageMovers report={data} />
        </>
      ) : null}
      <BudgetsPanel />
      {pricesOpen && settings.data ? (
        <UsageSettingsDialog
          open
          onOpenChange={setPricesOpen}
          data={settings.data}
        />
      ) : null}
    </PageTemplate>
  )
}
