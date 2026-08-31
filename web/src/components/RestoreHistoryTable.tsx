import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useRestoreHistory } from '../queries/restoreHistory'
import { formatDate } from '../lib/format'
import { StatusBadge } from './backupAttemptStatus'
import type { RestoreHistoryRecord } from '../types/restoreHistory'

// RestoreHistoryTable is BackupsSection's BackupHistoryTable's
// restore-direction counterpart: same shape (a status-first table, newest
// attempt on top, polling handled entirely by useRestoreHistory the same
// way useBackupHistory drives BackupHistoryTable), rendered only once at
// least one restore has ever been triggered, so a database nobody has
// ever restored shows nothing extra here rather than an empty table with
// nothing in it.
export function RestoreHistoryTable({ databaseName }: { databaseName: string }) {
  const { data, isLoading, error } = useRestoreHistory(databaseName)
  const history = data ?? []

  if (isLoading || history.length === 0) {
    return null
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-medium text-foreground">Restores</h3>
      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Status</TableHead>
              <TableHead>From backup</TableHead>
              <TableHead>Started</TableHead>
              <TableHead>Finished</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {history.map((record: RestoreHistoryRecord) => (
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
