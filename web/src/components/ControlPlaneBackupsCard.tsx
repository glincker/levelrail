import { useState } from 'react'
import {
  DatabaseIcon,
  DownloadSimpleIcon,
  KeyIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { formatAge, formatBytes } from '../lib/format'
import {
  controlPlaneBackupDownloadUrl,
  useControlPlaneBackups,
  useCreateControlPlaneBackup,
  useDeleteControlPlaneBackup,
} from '../queries/controlPlaneBackups'
import type { ControlPlaneBackup } from '../queries/controlPlaneBackups'

function BackupRow({
  backup,
  onDelete,
}: {
  backup: ControlPlaneBackup
  onDelete: (name: string) => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 py-2">
      <div className="min-w-0">
        <p className="truncate font-mono text-sm text-foreground">
          {backup.name}
        </p>
        <p className="text-xs text-muted-foreground">
          {formatBytes(backup.size_bytes)} · {formatAge(backup.created_at)} ·
          sha256 <span className="font-mono">{backup.sha256.slice(0, 12)}</span>
        </p>
      </div>
      <div className="flex shrink-0 gap-2">
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          render={
            <a href={controlPlaneBackupDownloadUrl(backup.name)} download />
          }
        >
          <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
          Download
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            onDelete(backup.name)
          }}
        >
          <TrashIcon className="size-3.5" aria-hidden="true" />
          Delete
        </Button>
      </div>
    </div>
  )
}

function DeleteBackupDialog({
  name,
  onClose,
}: {
  name: string | null
  onClose: () => void
}) {
  const del = useDeleteControlPlaneBackup()
  return (
    <Dialog
      open={name !== null}
      onOpenChange={(open) => {
        if (!open) {
          del.reset()
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Delete backup?</DialogTitle>
          <DialogDescription>
            <span className="font-mono">{name}</span> is permanently removed
            from this server. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {del.isError ? (
          <p className="text-sm text-destructive">{del.error.message}</p>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={del.isPending}
            onClick={() => {
              if (name === null) return
              del.mutate(name, {
                onSuccess: () => {
                  toast.add({ title: 'Backup deleted.', type: 'success' })
                  onClose()
                },
              })
            }}
          >
            {del.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function ControlPlaneBackupsCard() {
  const { data: backups, isLoading, isError } = useControlPlaneBackups()
  const create = useCreateControlPlaneBackup()
  const [deleting, setDeleting] = useState<string | null>(null)

  // Root-only endpoint: a 403 (or any list failure) hides the card.
  if (isLoading || isError || !backups) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <DatabaseIcon className="size-4" />
            </div>
            <div>
              <CardTitle>Control plane backups</CardTitle>
              <CardDescription>
                Consistent snapshots of this instance&apos;s own database.
              </CardDescription>
            </div>
          </div>
          <Button
            size="sm"
            disabled={create.isPending}
            onClick={() => {
              create.mutate(undefined, {
                onSuccess: (b) => {
                  toast.add({
                    title: `Backup ${b.name} created.`,
                    type: 'success',
                  })
                },
                onError: (e) => {
                  toast.add({ title: e.message, type: 'error' })
                },
              })
            }}
          >
            {create.isPending ? 'Backing up...' : 'Back up now'}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div
          role="note"
          className="flex items-start gap-2 rounded-md border border-border bg-muted/40 p-3 text-sm text-muted-foreground"
        >
          <KeyIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>
            The master key is not included in a backup. Secrets in the snapshot
            are encrypted with it, so a restore without the same master key
            leaves them unreadable. Store the key separately.
          </span>
        </div>
        {backups.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">
            No backups yet. Use &ldquo;Back up now&rdquo; to take the first one.
          </p>
        ) : (
          <div className="divide-y divide-border">
            {backups.map((b) => (
              <BackupRow key={b.name} backup={b} onDelete={setDeleting} />
            ))}
          </div>
        )}
        <p className="text-xs text-muted-foreground">
          Scheduled every 24h by default, keeping the newest 7 scheduled
          snapshots (APP_CONTROL_PLANE_BACKUP_INTERVAL, 0 disables;
          APP_CONTROL_PLANE_BACKUP_RETAIN). Manual backups are never deleted
          automatically.
        </p>
      </CardContent>
      <DeleteBackupDialog
        name={deleting}
        onClose={() => {
          setDeleting(null)
        }}
      />
    </Card>
  )
}
