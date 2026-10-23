import { Suspense, lazy, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { useDashboardTabs } from '@/hooks/useDashboardTabs'
import {
  METRICS_TAB,
  START_TAB,
  boardTab,
  metricsDoc,
  savedDashboardId,
  savedTabId,
} from '@/lib/dashboardTabs'
import { useDashboards } from '@/queries/customDashboards'
import { DashboardTabBar } from './DashboardTabBar'
import { ManageMenu } from './ManageMenu'
import { NewDashboardDialog } from './NewDashboardDialog'
import { RenameBoardDialog } from './RenameBoardDialog'
import { useBoardActions } from './useBoardActions'

// Non-Start tabs stay out of the home route chunk until selected.
const DashboardView = lazy(() =>
  import('@/components/dashboards/DashboardView').then((m) => ({
    default: m.DashboardView,
  })),
)
const MetricsView = lazy(() =>
  import('./MetricsView').then((m) => ({ default: m.MetricsView })),
)

/** Tab bar plus the active view: Start (passed in), the built-in Metrics, or a saved board. Only the shown tab mounts, so hidden boards fetch nothing. */
export function DashboardViews({
  start,
  hasApps,
  board,
}: {
  start: ReactNode
  hasApps: boolean
  /** From ?board=: a saved board id, or "start" / "metrics". */
  board?: string
}) {
  const { t } = useTranslation('dashboard')
  const list = useDashboards()
  const rows = list.data ?? []
  const tabs = useDashboardTabs(
    rows.map((d) => d.id),
    hasApps,
  )
  const [starterOpen, setStarterOpen] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [editing, setEditing] = useState(false)
  const actions = useBoardActions((id) => {
    tabs.select(savedTabId(id))
  })
  const { select } = tabs

  useEffect(() => {
    if (board) select(boardTab(board))
  }, [board, select])

  const labelOf = (id: string) => {
    if (id === START_TAB) return t('tabs.start')
    if (id === METRICS_TAB) return t('tabs.metrics')
    return rows.find((d) => d.id === savedDashboardId(id))?.name ?? id
  }

  // Saved tabs and the apps-based default stay unresolved until the list
  // arrives; avoid flashing the wrong tab.
  const waiting =
    list.isLoading &&
    (savedDashboardId(tabs.storedTab) !== null || (!tabs.hasStored && hasApps))
  const current = rows.find((d) => d.id === savedDashboardId(tabs.selected))

  let body: ReactNode = start
  if (waiting) body = <Skeleton className="h-64 w-full" />
  else if (tabs.selected === METRICS_TAB) {
    body = (
      <MetricsView
        busy={actions.busy}
        hasApps={hasApps}
        onStart={() => {
          tabs.select(START_TAB)
        }}
        onCustomize={() => {
          actions.create(metricsDoc(t('tabs.metrics')))
        }}
      />
    )
  } else if (current) {
    body = (
      <DashboardView
        key={current.id + current.updated_at}
        current={current}
        editing={editing}
        onEditingChange={setEditing}
      />
    )
  }

  return (
    <div className="space-y-6">
      <DashboardTabBar
        shown={tabs.shown}
        hidden={tabs.hidden}
        selected={tabs.selected}
        labelOf={labelOf}
        onSelect={(id) => {
          setEditing(false)
          tabs.select(id)
        }}
        onHide={tabs.hide}
        onShow={tabs.show}
        onMove={tabs.move}
        manage={
          <ManageMenu
            board={current ?? null}
            editing={editing}
            busy={actions.busy}
            onNew={() => {
              actions.create({
                version: 1,
                name: t('tabs.emptyName'),
                widgets: [],
              })
            }}
            onStarter={() => {
              setStarterOpen(true)
            }}
            onImportFile={(f) => void actions.importFile(f)}
            onEdit={() => {
              setEditing((e) => !e)
            }}
            onRename={() => {
              setRenaming(true)
            }}
            onExport={() => {
              if (current) actions.exportBoard(current)
            }}
            onDelete={() => {
              setDeleting(true)
            }}
          />
        }
      />
      <div
        role="tabpanel"
        id="dashboard-tabpanel"
        aria-labelledby={`dashboard-tab-${tabs.selected}`}
      >
        <Suspense fallback={<Skeleton className="h-64 w-full" />}>
          {body}
        </Suspense>
      </div>
      <NewDashboardDialog
        open={starterOpen}
        busy={actions.busy}
        emptyName={t('tabs.emptyName')}
        onOpenChange={setStarterOpen}
        onCreate={(doc) => {
          actions.create(doc, () => {
            setStarterOpen(false)
          })
        }}
      />
      {renaming && current ? (
        <RenameBoardDialog
          name={current.name}
          busy={actions.renaming}
          onOpenChange={setRenaming}
          onSave={(name) => {
            actions.rename(current, name, () => {
              setRenaming(false)
            })
          }}
        />
      ) : null}
      {current ? (
        <ConfirmDialog
          open={deleting}
          onOpenChange={setDeleting}
          title={t('manage.deleteTitle', { name: current.name })}
          description={t('manage.deleteBody')}
          confirmLabel={t('manage.delete')}
          tone="destructive"
          pending={actions.deleting}
          onConfirm={() => {
            actions.delete(current, () => {
              setDeleting(false)
            })
          }}
        />
      ) : null}
    </div>
  )
}
