import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { formatDate } from '../lib/format'
import { RestoreStatusCell } from './RestoreStatusCell'
import type { BaseBackupHistoryRecord } from '../types/pitr'

interface PITRRestoreRecord {
  id: string
  status: string
  error?: string
  target_timestamp: string
  started_at: string
  finished_at?: string
}

export function BaseBackupHistoryTableView({
  history,
  isLoading,
}: {
  history: BaseBackupHistoryRecord[]
  isLoading: boolean
}) {
  const { t } = useTranslation('backups')
  const columns = useMemo<ResponsiveColumn<BaseBackupHistoryRecord>[]>(
    () => [
      {
        id: 'status',
        header: 'Status',
        primary: true,
        cell: (r) => <RestoreStatusCell status={r.status} error={r.error} />,
      },
      {
        id: 'size',
        header: 'Size',
        cell: (r) => (
          <span className="text-muted-foreground">
            {r.size_bytes > 0
              ? `${(r.size_bytes / 1024 / 1024).toFixed(1)} MiB`
              : '-'}
          </span>
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
  return (
    <ResponsiveTable
      rows={history}
      columns={columns}
      getRowId={(r) => r.id}
      ariaLabel={t('tables.pitrBackups')}
    />
  )
}

export function PITRRestoreHistoryTableView({
  history,
  isLoading,
}: {
  history: PITRRestoreRecord[]
  isLoading: boolean
}) {
  const { t } = useTranslation('backups')
  const columns = useMemo<ResponsiveColumn<PITRRestoreRecord>[]>(
    () => [
      {
        id: 'status',
        header: 'Status',
        primary: true,
        cell: (r) => (
          <RestoreStatusCell
            status={r.status as 'running' | 'succeeded' | 'failed'}
            error={r.error}
          />
        ),
      },
      {
        id: 'target',
        header: 'Target time',
        cell: (r) => (
          <span className="text-muted-foreground">
            {formatDate(r.target_timestamp, '-')}
          </span>
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
  return (
    <ResponsiveTable
      rows={history}
      columns={columns}
      getRowId={(r) => r.id}
      ariaLabel={t('tables.pitrRestores')}
    />
  )
}
