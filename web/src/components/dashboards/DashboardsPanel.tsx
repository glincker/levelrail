import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useCreateDashboard, useDashboards } from '@/queries/customDashboards'
import type { DashboardDoc } from '@/types/dashboards'
import { DashboardView } from './DashboardView'
import { DashboardList } from './DashboardList'
import { TemplatePicker } from './TemplatePicker'

/** The dashboards page body: the list on the left, the selected dashboard on the right, starter templates when empty. */
export function DashboardsPanel({
  selectedId,
  onSelect,
}: {
  selectedId: string
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation('dashboards')
  const list = useDashboards()
  const create = useCreateDashboard()
  const rows = useMemo(() => list.data ?? [], [list.data])
  const current = rows.find((d) => d.id === selectedId) ?? rows[0]

  function createFrom(doc: DashboardDoc) {
    create.mutate(
      { scope: 'user', doc },
      {
        onSuccess: (d) => {
          onSelect(d.id)
          toast.add({
            title: t('toast.created', { name: d.name }),
            type: 'success',
          })
        },
        onError: (e) =>
          toast.add({
            title: t('toast.createFailed'),
            description: e.message,
            type: 'error',
          }),
      },
    )
  }

  if (list.isLoading) return <Skeleton className="h-64 w-full" />
  if (list.error) {
    return <p className="text-sm text-destructive">{list.error.message}</p>
  }
  if (!current) {
    return (
      <div className="space-y-6">
        <EmptyState
          compact
          icon={<SquaresFourIcon className="size-5" />}
          title={t('empty.title')}
          description={t('empty.body')}
        />
        <TemplatePicker busy={create.isPending} onPick={createFrom} />
      </div>
    )
  }
  return (
    <div className="grid gap-6 lg:grid-cols-[14rem_1fr]">
      <aside className="space-y-4">
        <DashboardList
          dashboards={rows}
          selectedId={current.id}
          onSelect={onSelect}
        />
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground">
            {t('template.new')}
          </summary>
          <div className="mt-3">
            <TemplatePicker
              compact
              busy={create.isPending}
              onPick={createFrom}
            />
          </div>
        </details>
      </aside>
      <DashboardView key={current.id + current.updated_at} current={current} />
    </div>
  )
}
