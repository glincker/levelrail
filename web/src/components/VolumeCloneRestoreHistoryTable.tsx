import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useVolumeCloneRestores } from '../queries/volumeCloneRestore'
import { formatDate } from '../lib/format'
import { StatusBadge } from './backupAttemptStatus'
import type { VolumeCloneRestoreRecord } from '../types/volumeCloneRestore'

// VolumeCloneRestoreHistoryTable is CloneRestoreHistoryTable's app service
// volume counterpart: same shape (status-first, newest attempt on top,
// self-terminating polling), rendered only once at least one clone-restore
// has ever been triggered for this volume. Unlike CloneRestoreHistoryTable
// the new volume's name is plain text, not a link: a standalone Docker
// volume has no detail page anywhere in this app today, so there is
// nowhere to link to (see AppVolumeBackupsSection's own header comment for
// this limitation).
export function VolumeCloneRestoreHistoryTable({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const { data, isLoading, error } = useVolumeCloneRestores(
    appName,
    volumeName,
  )
  const history = data ?? []

  if (isLoading || history.length === 0) {
    return null
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-medium text-foreground">
        Restored as new volume
      </h3>
      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Status</TableHead>
              <TableHead>New volume</TableHead>
              <TableHead>From backup</TableHead>
              <TableHead>Started</TableHead>
              <TableHead>Finished</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {history.map((record: VolumeCloneRestoreRecord) => (
              <TableRow key={record.id}>
                <TableCell>
                  <div className="flex flex-col gap-1">
                    <StatusBadge status={record.status} />
                    {record.status === 'failed' && record.error ? (
                      <span
                        className="max-w-[20rem] truncate text-xs text-destructive"
                        title={record.error}
                      >
                        {record.error}
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className="font-mono text-xs text-foreground">
                  {record.new_volume_name}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {record.backup_history_id}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(record.started_at, '-')}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(record.finished_at, '-')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
