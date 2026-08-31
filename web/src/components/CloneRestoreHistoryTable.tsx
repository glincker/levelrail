import { Link } from '@tanstack/react-router'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useCloneRestores } from '../queries/cloneRestore'
import { formatDate } from '../lib/format'
import { StatusBadge } from './backupAttemptStatus'
import type { CloneRestoreRecord } from '../types/cloneRestore'

// CloneRestoreHistoryTable is RestoreHistoryTable's "restore as new
// database" counterpart: same shape (status-first, newest attempt on
// top, self-terminating polling via useCloneRestores), rendered only
// once at least one clone-restore has ever been triggered from this
// database, the same "nothing extra for a database nobody has restored"
// convention RestoreHistoryTable already follows. The new database's own
// name links to its detail page once the row exists, succeeded or not:
// even a still-running or failed attempt already created the database
// row itself (createDesiredDatabase runs synchronously before the
// background restore starts), so the link is never dead.
export function CloneRestoreHistoryTable({
  databaseName,
}: {
  databaseName: string
}) {
  const { data, isLoading, error } = useCloneRestores(databaseName)
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
        Restored as new database
      </h3>
      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Status</TableHead>
              <TableHead>New database</TableHead>
              <TableHead>From backup</TableHead>
              <TableHead>Started</TableHead>
              <TableHead>Finished</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {history.map((record: CloneRestoreRecord) => (
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
                <TableCell>
                  <Link
                    to="/databases/$name"
                    params={{ name: record.new_database_name }}
                    className="text-primary underline underline-offset-2"
                  >
                    {record.new_database_name}
                  </Link>
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
