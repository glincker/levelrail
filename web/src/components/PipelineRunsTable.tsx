import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { usePipelineRuns } from '../queries/pipelines'
import { formatDuration } from '../lib/pipelineStatus'
import { PipelineReportLink } from './PipelineReportLink'
import { PipelineStatusBadge } from './PipelineStatusBadge'

export function PipelineRunsTable({
  appName,
  pipeline,
}: {
  appName: string
  pipeline?: string
}) {
  const { t } = useTranslation('backups')
  const { data, isLoading, error } = usePipelineRuns(appName, pipeline)
  type Run = NonNullable<typeof data>[number]
  const columns = useMemo<ResponsiveColumn<Run>[]>(
    () => [
      {
        id: 'run',
        header: 'Run',
        primary: true,
        cell: (r) => (
          <Link
            to="/apps/$name/pipelines/runs/$runId"
            params={{ name: appName, runId: r.id }}
            className="font-medium text-foreground underline-offset-2 hover:underline"
          >
            {r.pipeline_name} #{r.number}
          </Link>
        ),
      },
      {
        id: 'status',
        header: 'Status',
        cell: (r) => <PipelineStatusBadge status={r.status} />,
      },
      {
        id: 'trigger',
        header: 'Trigger',
        priority: 2,
        cell: (r) => (
          <span className="text-xs text-muted-foreground">
            {r.trigger}
            {r.actor ? ` by ${r.actor}` : ''}
          </span>
        ),
      },
      {
        id: 'ref',
        header: 'Ref',
        priority: 2,
        cell: (r) => (
          <span className="block max-w-48 truncate font-mono text-xs text-muted-foreground">
            {r.ref ?? ''}
          </span>
        ),
      },
      {
        id: 'started',
        header: 'Started',
        priority: 3,
        cell: (r) => (
          <span className="text-xs text-muted-foreground">
            {new Date(r.created_at).toLocaleString()}
          </span>
        ),
      },
      {
        id: 'duration',
        header: 'Duration',
        priority: 3,
        cell: (r) => (
          <span className="text-xs tabular-nums text-muted-foreground">
            {formatDuration(r.started_at, r.finished_at)}
          </span>
        ),
      },
      {
        id: 'forge',
        header: 'Forge status',
        priority: 3,
        cell: (r) => (
          <span className="text-xs">
            <PipelineReportLink report={r.report} />
          </span>
        ),
      },
    ],
    [appName],
  )
  if (isLoading) {
    return <TableSkeleton columnCount={7} rowCount={4} />
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }
  const runs = data ?? []
  if (runs.length === 0) {
    return <p className="text-sm text-muted-foreground">No runs yet.</p>
  }
  return (
    <ResponsiveTable
      rows={runs}
      columns={columns}
      getRowId={(r) => r.id}
      ariaLabel={t('tables.pipelineRuns')}
    />
  )
}
