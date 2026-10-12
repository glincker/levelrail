import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useRestoreHistory } from '../queries/restoreHistory'
import { formatDate } from '../lib/format'
import { RestoreStatusCell } from './RestoreStatusCell'
import type { RestoreHistoryRecord } from '../types/restoreHistory'

// Shared by a database's restore attempts and AppVolumeBackupsSection's volume
// restores; rendered only once a restore has ever been triggered.
export function RestoreHistoryTableView({
  history,
  isLoading,
  error,
}: {
  history: RestoreHistoryRecord[]
  isLoading: boolean
  error: Error | null
}) {
  const { t } = useTranslation('backups')
  const columns = useMemo<ResponsiveColumn<RestoreHistoryRecord>[]>(
    () => [
      {
        id: 'status',
        header: 'Status',
        primary: true,
        cell: (r) => <RestoreStatusCell status={r.status} error={r.error} />,
      },
      {
        id: 'backup',
        header: 'From backup',
        cell: (r) => (
          <span className="text-muted-foreground">{r.backup_history_id}</span>
        ),
      },
      {
        id: 'started',
        header: 'Started',
        priority: 2,
        cell: (r) => (
          <span className="text-muted-foreground">
            {formatDate(r.started_at, '-')}
          </span>
        ),
      },
      {
        id: 'finished',
        header: 'Finished',
        priority: 3,
        cell: (r) => (
          <span className="text-muted-foreground">
            {formatDate(r.finished_at, '-')}
          </span>
        ),
      },
    ],
    [],
  )

  if (isLoading || history.length === 0) {
    return null
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-medium text-foreground">Restores</h3>
      <ResponsiveTable
        rows={history}
        columns={columns}
        getRowId={(r) => r.id}
        ariaLabel={t('tables.restores')}
      />
    </div>
  )
}

export function RestoreHistoryTable({
  databaseName,
}: {
  databaseName: string
}) {
  const { data, isLoading, error } = useRestoreHistory(databaseName)
  return (
    <RestoreHistoryTableView
      history={data ?? []}
      isLoading={isLoading}
      error={error}
    />
  )
}
