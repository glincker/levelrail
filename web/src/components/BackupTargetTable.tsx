import {
  CloudArrowUpIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { DeleteBackupTargetDialog } from './DeleteBackupTargetDialog'
import { PROVIDER_LABEL } from './backupTargetProvider'
import { useTestBackupTarget } from '../queries/backupTargets'
import type { BackupTarget } from '../types/backupTarget'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Mirrors RegistryCredentialTable.tsx's own TestButton exactly: same
// loading/success/failure states, same toast pattern, for the
// identical "verify a stored credential by actually using it, once, on
// demand" action against a different resource.
function TestButton({ target }: { target: BackupTarget }) {
  const testTarget = useTestBackupTarget()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={testTarget.isPending}
      onClick={() => {
        testTarget.mutate(target.id, {
          onSuccess: () => {
            toast.add({
              title: `"${target.name}" connected successfully.`,
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: `"${target.name}" failed to connect.`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {testTarget.isPending ? 'Testing...' : 'Test connection'}
    </Button>
  )
}

// Only the destination itself: bucket, provider, region/endpoint,
// connected date. No column for backup runs or last-backup status,
// since GET /api/v1/backup-targets carries none of that (nothing writes
// backup history yet, see queries/backupTargets.ts's own header
// comment), rather than a column that would just be permanently empty.
export function BackupTargetTable({ targets }: { targets: BackupTarget[] }) {
  if (targets.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<CloudArrowUpIcon className="size-5" />}
        title="No backup targets connected"
        description="Connect an S3-compatible bucket (AWS S3, Cloudflare R2, or any other S3-compatible endpoint) to use as a backup destination for managed databases."
      />
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
                <div className="flex justify-end gap-2">
                  <TestButton target={target} />
                  <DeleteBackupTargetDialog target={target} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
