import { Skeleton } from './skeleton'
import { Table, TableBody, TableCell, TableRow } from './table'

// Route pendingComponent fallback for the account-level settings tables
// (tokens, backup targets, notification channels, registry credentials,
// users, audit log): a fixed, small row count rather than mirroring
// whatever the real list ends up being, matching RowSkeleton's own
// "don't try to virtualize the skeleton itself" reasoning.
export function TableSkeleton({
  columnCount,
  rowCount = 5,
}: {
  columnCount: number
  rowCount?: number
}) {
  return (
    <div className="rounded-lg border border-border" aria-hidden="true">
      <Table>
        <TableBody>
          {Array.from({ length: rowCount }, (_, row) => (
            <TableRow key={row}>
              {Array.from({ length: columnCount }, (_, col) => (
                <TableCell key={col}>
                  <Skeleton className="h-4 w-full" />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
