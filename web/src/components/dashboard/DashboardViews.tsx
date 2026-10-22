import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DashboardView } from '@/components/dashboards/DashboardView'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useDashboardTabs } from '@/hooks/useDashboardTabs'
import {
  METRICS_TAB,
  START_TAB,
  metricsDoc,
  savedDashboardId,
  savedTabId,
} from '@/lib/dashboardTabs'
import { useCreateDashboard, useDashboards } from '@/queries/customDashboards'
import type { DashboardDoc } from '@/types/dashboards'
import { DashboardTabBar } from './DashboardTabBar'
import { MetricsView } from './MetricsView'
import { NewDashboardDialog } from './NewDashboardDialog'

/** Tab bar plus the active view: Start (passed in), the built-in Metrics, or a saved dashboard. */
export function DashboardViews({ start }: { start: ReactNode }) {
  const { t } = useTranslation('dashboard')
  const list = useDashboards()
  const create = useCreateDashboard()
  const rows = list.data ?? []
  const tabs = useDashboardTabs(rows.map((d) => d.id))
  const [adding, setAdding] = useState(false)

  const labelOf = (id: string) => {
    if (id === START_TAB) return t('tabs.start')
    if (id === METRICS_TAB) return t('tabs.metrics')
    return rows.find((d) => d.id === savedDashboardId(id))?.name ?? id
  }

  function createFrom(doc: DashboardDoc, failed: string) {
    create.mutate(
      { scope: 'user', doc },
      {
        onSuccess: (d) => {
          setAdding(false)
          tabs.select(savedTabId(d.id))
        },
        onError: (e) => {
          toast.add({ title: failed, description: e.message, type: 'error' })
        },
      },
    )
  }

  // A saved tab stays unresolved until the list arrives; avoid flashing Start.
  const waiting = list.isLoading && savedDashboardId(tabs.storedTab) !== null
  const current = rows.find((d) => d.id === savedDashboardId(tabs.selected))

  let body: ReactNode = start
  if (waiting) body = <Skeleton className="h-64 w-full" />
  else if (tabs.selected === METRICS_TAB) {
    body = (
      <MetricsView
        busy={create.isPending}
        onCustomize={() => {
          createFrom(metricsDoc(t('tabs.metrics')), t('tabs.copyFailed'))
        }}
      />
    )
  } else if (current) {
    body = (
      <DashboardView key={current.id + current.updated_at} current={current} />
    )
  }

  return (
    <div className="space-y-6">
      <DashboardTabBar
        shown={tabs.shown}
        hidden={tabs.hidden}
        selected={tabs.selected}
        labelOf={labelOf}
        onSelect={tabs.select}
        onAdd={() => {
          setAdding(true)
        }}
        onHide={tabs.hide}
        onShow={tabs.show}
        onMove={tabs.move}
      />
      <div
        role="tabpanel"
        id="dashboard-tabpanel"
        aria-labelledby={`dashboard-tab-${tabs.selected}`}
      >
        {body}
      </div>
      <NewDashboardDialog
        open={adding}
        busy={create.isPending}
        emptyName={t('tabs.emptyName')}
        onOpenChange={setAdding}
        onCreate={(doc) => {
          createFrom(doc, t('tabs.createFailed'))
        }}
      />
    </div>
  )
}
