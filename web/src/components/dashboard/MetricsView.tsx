import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChartLineUpIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { DashboardGrid } from '@/components/dashboards/DashboardGrid'
import { WidgetBody } from '@/components/dashboards/WidgetBody'
import { metricsDoc } from '@/lib/dashboardTabs'
import { resolveTimeRange } from '@/lib/timeRange'

const noop = () => undefined

/** Built-in read-only fleet dashboard; Customize copies it into a saved dashboard. */
export function MetricsView({
  busy,
  hasApps,
  onCustomize,
  onStart,
}: {
  busy: boolean
  hasApps: boolean
  onCustomize: () => void
  onStart: () => void
}) {
  const { t } = useTranslation('dashboard')
  const doc = useMemo(() => metricsDoc(t('tabs.metrics')), [t])
  const range = useMemo(() => resolveTimeRange(doc.time_range ?? '24h'), [doc])
  if (!hasApps) {
    return (
      <EmptyState
        icon={<ChartLineUpIcon className="size-5" />}
        title={t('tabs.metricsEmptyTitle')}
        description={t('tabs.metricsEmptyBody')}
        action={
          <Button onClick={onStart}>{t('tabs.metricsEmptyAction')}</Button>
        }
      />
    )
  }
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t('tabs.metricsNote')}</p>
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={onCustomize}
        >
          <CopyIcon />
          {t('tabs.customize')}
        </Button>
      </div>
      <DashboardGrid
        widgets={doc.widgets}
        editing={false}
        onChange={noop}
        renderBody={(w) => <WidgetBody w={w} range={range} />}
      />
    </div>
  )
}
