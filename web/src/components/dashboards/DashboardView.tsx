import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { addWidget } from '@/lib/dashboardGrid'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type TimeRangeKey,
} from '@/lib/timeRange'
import { useUpdateDashboard } from '@/queries/customDashboards'
import type {
  CustomDashboard,
  DashboardDoc,
  DashboardWidget,
} from '@/types/dashboards'
import { AddWidgetDialog } from './AddWidgetDialog'
import { DashboardGrid } from './DashboardGrid'
import { DashboardToolbar, type Placement } from './DashboardToolbar'
import { WidgetBody } from './WidgetBody'

/** One saved dashboard: range and refresh controls, widget grid and add dialog, with local edits until saved. Edit mode is driven by the Manage menu. */
export function DashboardView({
  current,
  editing,
  onEditingChange,
}: {
  current: CustomDashboard
  editing: boolean
  onEditingChange: (editing: boolean) => void
}) {
  const { t } = useTranslation('dashboards')
  const update = useUpdateDashboard()
  const [widgets, setWidgets] = useState<DashboardWidget[]>(current.doc.widgets)
  const [timeRange, setTimeRange] = useState<TimeRangeKey>(
    current.doc.time_range ?? DEFAULT_TIME_RANGE_KEY,
  )
  const [refreshSeconds, setRefreshSeconds] = useState(
    current.doc.refresh_seconds ?? 0,
  )
  const [placement, setPlacement] = useState<Placement>({
    scope: current.scope,
    projectId: current.project_id ?? '',
    isDefault: current.is_default,
  })
  const [now, setNow] = useState(() => new Date())
  const [adding, setAdding] = useState(false)

  useEffect(() => {
    if (refreshSeconds <= 0) return
    const id = window.setInterval(
      () => setNow(new Date()),
      refreshSeconds * 1000,
    )
    return () => window.clearInterval(id)
  }, [refreshSeconds])

  const range = useMemo(
    () => resolveTimeRange(timeRange, now),
    [timeRange, now],
  )
  const doc: DashboardDoc = {
    ...current.doc,
    time_range: timeRange,
    refresh_seconds: refreshSeconds,
    widgets,
  }
  const dirty =
    JSON.stringify(doc) !== JSON.stringify(current.doc) ||
    placement.scope !== current.scope ||
    placement.projectId !== (current.project_id ?? '') ||
    placement.isDefault !== current.is_default

  function save() {
    update.mutate(
      {
        id: current.id,
        body: {
          scope: placement.scope,
          project_id: placement.projectId || undefined,
          is_default: placement.isDefault,
          doc,
        },
      },
      {
        onSuccess: () =>
          toast.add({ title: t('toast.saved'), type: 'success' }),
        onError: (e) =>
          toast.add({
            title: t('toast.saveFailed'),
            description: e.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <div className="space-y-4">
      <DashboardToolbar
        editing={editing}
        dirty={dirty}
        saving={update.isPending}
        timeRange={timeRange}
        refreshSeconds={refreshSeconds}
        placement={placement}
        onTimeRange={setTimeRange}
        onRefreshSeconds={setRefreshSeconds}
        onRefreshNow={() => setNow(new Date())}
        onPlacement={setPlacement}
        onDoneEditing={() => onEditingChange(false)}
        onAdd={() => setAdding(true)}
        onSave={save}
      />
      {widgets.length === 0 ? (
        <EmptyState
          compact
          icon={<LayoutGridGlassIcon className="size-6" />}
          title={t('empty.widgetsTitle')}
          description={t('empty.widgetsBody')}
        />
      ) : (
        <DashboardGrid
          widgets={widgets}
          editing={editing}
          onChange={setWidgets}
          renderBody={(w) => <WidgetBody w={w} range={range} />}
        />
      )}
      <AddWidgetDialog
        open={adding}
        onOpenChange={setAdding}
        existing={widgets}
        onAdd={(w) => setWidgets((cur) => addWidget(cur, w))}
      />
    </div>
  )
}
