import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import {
  ROW_LINK_OVERLAY,
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { AppListEntry } from '../../types/appDetail'
import { PlacementLabel } from '../PlacementLabel'
import { AppRowActions } from '../AppRowActions'
import { StatusLabel } from '../AppRow'
import { AppLogo, AppMetaChips } from './AppMetaChips'
import {
  ErrorCell,
  LastDeployCell,
  P95Cell,
  TrafficSpark,
} from './AppMetricCells'
import { AppQuickActions } from './AppQuickActions'
import { useAppRowMetrics } from './useAppRowMetrics'

type MetricKind = 'traffic' | 'p95' | 'errors' | 'deployed'

// One query per app (deduped by key) feeds all four metric columns.
function MetricCell({ app, kind }: { app: AppListEntry; kind: MetricKind }) {
  const metrics = useAppRowMetrics(app.name, true)
  switch (kind) {
    case 'traffic':
      return <TrafficSpark metrics={metrics} name={app.name} />
    case 'p95':
      return <P95Cell metrics={metrics} />
    case 'errors':
      return <ErrorCell metrics={metrics} />
    case 'deployed':
      return <LastDeployCell metrics={metrics} />
  }
}

export function AppsTable({
  apps,
  selected,
  onSelect,
}: {
  apps: readonly AppListEntry[]
  selected: readonly string[]
  onSelect: (name: string, checked: boolean) => void
}) {
  const { t } = useTranslation('common')
  const columns = useMemo<ResponsiveColumn<AppListEntry>[]>(
    () => [
      {
        id: 'name',
        primary: true,
        header: t('appsList.colName'),
        cell: (app) => (
          <span className="flex min-w-0 flex-col gap-1">
            <Link
              to="/apps/$name"
              params={{ name: app.name }}
              className={`truncate text-sm font-medium text-foreground ${ROW_LINK_OVERLAY}`}
            >
              {app.name}
            </Link>
            <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <StatusLabel status={app.status} />
              <AppMetaChips app={app} />
            </span>
          </span>
        ),
      },
      {
        id: 'node',
        header: t('appsList.colNode'),
        priority: 2,
        width: 'w-32',
        cell: (app) => (
          <PlacementLabel
            nodeId={app.node_id}
            nodeName={app.node_name}
            nodeIsLocal={app.node_is_local}
            className="text-sm"
          />
        ),
      },
      {
        id: 'traffic',
        header: t('appsList.colTraffic'),
        priority: 2,
        width: 'w-28',
        cell: (app) => <MetricCell app={app} kind="traffic" />,
      },
      {
        id: 'p95',
        header: t('appsList.colP95'),
        priority: 2,
        width: 'w-20',
        cell: (app) => <MetricCell app={app} kind="p95" />,
      },
      {
        id: 'errors',
        header: t('appsList.colErrors'),
        priority: 3,
        width: 'w-20',
        cell: (app) => <MetricCell app={app} kind="errors" />,
      },
      {
        id: 'deployed',
        header: t('appsList.colDeployed'),
        priority: 3,
        width: 'w-28',
        cell: (app) => <MetricCell app={app} kind="deployed" />,
      },
    ],
    [t],
  )
  return (
    <ResponsiveTable
      rows={apps}
      columns={columns}
      getRowId={(app) => app.name}
      ariaLabel={t('appsList.tableLabel')}
      leading={{
        header: <span className="sr-only">{t('foundation.table.select')}</span>,
        width: 'w-24',
        cell: (app) => (
          <span className="relative z-10 flex items-center gap-3">
            <Checkbox
              checked={selected.includes(app.name)}
              aria-label={t('appsList.selectApp', { name: app.name })}
              onCheckedChange={(checked) => {
                onSelect(app.name, checked)
              }}
            />
            <AppLogo image={app.image} />
          </span>
        ),
      }}
      renderActions={(app) => (
        <span className="flex items-center">
          <AppQuickActions app={app} />
          <AppRowActions app={app} />
        </span>
      )}
    />
  )
}
