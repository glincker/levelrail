import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { addWidget } from '@/lib/dashboardGrid'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type TimeRangeKey,
} from '@/lib/timeRange'
import {
  useDeleteDashboard,
  useImportDashboard,
  useUpdateDashboard,
} from '@/queries/customDashboards'
import type {
  CustomDashboard,
  DashboardDoc,
  DashboardWidget,
} from '@/types/dashboards'
import { AddWidgetDialog } from './AddWidgetDialog'
import { DashboardGrid } from './DashboardGrid'
import { DashboardToolbar, type Placement } from './DashboardToolbar'
import { WidgetBody } from './WidgetBody'

function downloadJson(name: string, doc: DashboardDoc) {
  const blob = new Blob([JSON.stringify(doc, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name.replace(/[^a-z0-9-]+/gi, '-').toLowerCase()}.json`
  a.click()
  URL.revokeObjectURL(url)
}

/** One saved dashboard: toolbar, widget grid, add dialog and delete confirm, with local edits until saved. */
export function DashboardView({ current }: { current: CustomDashboard }) {
  const { t } = useTranslation('dashboards')
  const update = useUpdateDashboard()
  const remove = useDeleteDashboard()
  const importer = useImportDashboard()
  const [editing, setEditing] = useState(false)
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
  const [confirmDelete, setConfirmDelete] = useState(false)

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

  async function importFile(file: File) {
    try {
      const parsed = JSON.parse(await file.text()) as DashboardDoc
      importer.mutate(parsed, {
        onSuccess: (d) =>
          toast.add({
            title: t('toast.imported', { name: d.name }),
            type: 'success',
          }),
        onError: (e) =>
          toast.add({
            title: t('toast.importFailed'),
            description: e.message,
            type: 'error',
          }),
      })
    } catch {
      toast.add({
        title: t('toast.importFailed'),
        description: t('toast.notJson'),
        type: 'error',
      })
    }
  }

  return (
    <div className="space-y-4">
      <DashboardToolbar
        current={current}
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
        onToggleEdit={() => setEditing((e) => !e)}
        onAdd={() => setAdding(true)}
        onSave={save}
        onExport={() => downloadJson(current.name, doc)}
        onImportFile={(f) => void importFile(f)}
        onDelete={() => setConfirmDelete(true)}
      />
      {widgets.length === 0 ? (
        <EmptyState
          compact
          icon={<SquaresFourIcon className="size-5" />}
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
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t('delete.title', { name: current.name })}
        description={t('delete.body')}
        confirmLabel={t('delete.confirm')}
        tone="destructive"
        pending={remove.isPending}
        onConfirm={() =>
          remove.mutate(current.id, {
            onSuccess: () => {
              setConfirmDelete(false)
              toast.add({ title: t('toast.deleted'), type: 'success' })
            },
            onError: (e) =>
              toast.add({
                title: t('toast.deleteFailed'),
                description: e.message,
                type: 'error',
              }),
          })
        }
      />
    </div>
  )
}
