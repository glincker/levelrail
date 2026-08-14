import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { DeleteBackupTargetDialog } from './DeleteBackupTargetDialog'
import { PROVIDER_LABEL } from './backupTargetProvider'
import type { BackupTarget } from '../types/backupTarget'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Only the destination itself: bucket, provider, region/endpoint,
// connected date. No column for backup runs or last-backup status,
// since GET /api/v1/backup-targets carries none of that (nothing writes
// backup history yet, see queries/backupTargets.ts's own header
// comment), rather than a column that would just be permanently empty.
export function BackupTargetTable({ targets }: { targets: BackupTarget[] }) {
  if (targets.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center">
        <div className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <CloudArrowUpIcon className="size-5" />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">
            No backup targets connected
          </p>
          <p className="text-sm text-muted-foreground">
            Connect an S3-compatible bucket to use as a backup destination for
            managed databases.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead>Bucket</TableHead>
            <TableHead>Region / endpoint</TableHead>
            <TableHead>Connected</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {targets.map((target) => (
            <TableRow key={target.id}>
              <TableCell className="font-medium text-foreground">
                {target.name}
              </TableCell>
              <TableCell>
                <Badge variant="outline">
                  {PROVIDER_LABEL[target.provider]}
                </Badge>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {target.bucket}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {target.region || target.endpoint ? (
                  <div className="flex flex-col gap-0.5">
                    {target.region ? <span>{target.region}</span> : null}
                    {target.endpoint ? (
                      <span className="text-xs break-all">
                        {target.endpoint}
                      </span>
                    ) : null}
                  </div>
                ) : (
                  '-'
                )}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDate(target.created_at)}
              </TableCell>
              <TableCell className="text-right">
                <DeleteBackupTargetDialog target={target} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
