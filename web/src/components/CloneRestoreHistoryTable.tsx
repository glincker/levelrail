import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useCloneRestores } from '../queries/cloneRestore'
import { formatDate } from '../lib/format'
import { RestoreStatusCell } from './RestoreStatusCell'
import type { CloneRestoreRecord } from '../types/cloneRestore'

// Rendered only once a clone-restore has been triggered. The new database row
// exists as soon as the attempt starts, so its link is never dead.
export function CloneRestoreHistoryTable({
  databaseName,
}: {
  databaseName: string
}) {
  const { t } = useTranslation('backups')
  const { data, isLoading, error } = useCloneRestores(databaseName)
  const history = data ?? []

  const columns = useMemo<ResponsiveColumn<CloneRestoreRecord>[]>(
    () => [
      {
        id: 'status',
        header: 'Status',
        primary: true,
        cell: (r) => <RestoreStatusCell status={r.status} error={r.error} />,
      },
      {
        id: 'new',
        header: 'New database',
        cell: (r) => (
          <Link
            to="/databases/$name"
            params={{ name: r.new_database_name }}
            className="text-primary underline underline-offset-2"
          >
            {r.new_database_name}
          </Link>
        ),
      },
      {
        id: 'backup',
        header: 'From backup',
        priority: 2,
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
      <h3 className="text-sm font-medium text-foreground">
        Restored as new database
      </h3>
      <ResponsiveTable
        rows={history}
        columns={columns}
        getRowId={(r) => r.id}
        ariaLabel={t('tables.cloneDatabases')}
      />
    </div>
  )
}
