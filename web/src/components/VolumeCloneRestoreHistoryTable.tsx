import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useVolumeCloneRestores } from '../queries/volumeCloneRestore'
import { formatDate } from '../lib/format'
import { RestoreStatusCell } from './RestoreStatusCell'
import type { VolumeCloneRestoreRecord } from '../types/volumeCloneRestore'

// The new volume name is plain text: a standalone Docker volume has no detail
// page to link to.
export function VolumeCloneRestoreHistoryTable({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const { t } = useTranslation('backups')
  const { data, isLoading, error } = useVolumeCloneRestores(appName, volumeName)
  const history = data ?? []

  const columns = useMemo<ResponsiveColumn<VolumeCloneRestoreRecord>[]>(
    () => [
      {
        id: 'status',
        header: 'Status',
        primary: true,
        cell: (r) => <RestoreStatusCell status={r.status} error={r.error} />,
      },
      {
        id: 'new',
        header: 'New volume',
        cell: (r) => (
          <span className="font-mono text-xs text-foreground">
            {r.new_volume_name}
          </span>
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
      <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
        Restored as new volume
      </h3>
      <ResponsiveTable
        rows={history}
        columns={columns}
        getRowId={(r) => r.id}
        ariaLabel={t('tables.cloneVolumes')}
      />
    </div>
  )
}
