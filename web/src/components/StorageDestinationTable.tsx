import { useState, type ReactNode } from 'react'
import {
  CloudArrowUpIcon,
  PlugsConnectedIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { BrandLogoBadge } from './BrandLogoBadge'
import {
  logoIdForStoragePreset,
  STORAGE_PRESET_LABEL,
} from '../lib/storageProviders'
import {
  useDeleteStorageDestination,
  useTestStorageDestination,
} from '../queries/storage'
import type { StorageDestination, StorageProbeResult } from '../types/storage'

function describeProbe(result: StorageProbeResult): string {
  const steps = result.steps
    .map((s) => `${s.name}: ${s.ok ? 'ok' : 'failed'}`)
    .join(', ')
  return result.ok ? steps : `${result.message ?? 'failed'} (${steps})`
}

function TestButton({ dest }: { dest: StorageDestination }) {
  const test = useTestStorageDestination()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={test.isPending}
      onClick={() => {
        test.mutate(dest.id, {
          onSuccess: (result) => {
            toast.add({
              title: result.ok
                ? `"${dest.name}" is working.`
                : `"${dest.name}" failed the connection test.`,
              description: describeProbe(result),
              type: result.ok ? 'success' : 'error',
            })
          },
          onError: (error) => {
            toast.add({
              title: `"${dest.name}" could not be tested.`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {test.isPending ? 'Testing...' : 'Test'}
    </Button>
  )
}

function DeleteButton({ dest }: { dest: StorageDestination }) {
  const [open, setOpen] = useState(false)
  const del = useDeleteStorageDestination()
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) del.reset()
      }}
    >
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        Delete
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Delete &ldquo;{dest.name}&rdquo;?</DialogTitle>
          <DialogDescription>
            This removes the connection. Objects already in the bucket are not
            deleted. Log archive policies using it must be removed first.
          </DialogDescription>
        </DialogHeader>
        {del.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {del.error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={del.isPending}
            onClick={() => {
              del.mutate(dest.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `"${dest.name}" disconnected.`,
                    type: 'success',
                  })
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

export function StorageDestinationTable({
  destinations,
  action,
}: {
  destinations: StorageDestination[]
  action?: ReactNode
}) {
  if (destinations.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<CloudArrowUpIcon className="size-5" />}
        title="No storage destinations"
        description="Connect AWS S3, Cloudflare R2, Backblaze B2, MinIO, Wasabi, or any S3-compatible bucket to archive logs and store backups."
        action={action}
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
            <TableHead>Archive policies</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {destinations.map((d) => (
            <TableRow key={d.id}>
              <TableCell className="font-medium text-foreground">
                {d.name}
              </TableCell>
              <TableCell>
                <Badge variant="outline" className="gap-1.5">
                  <BrandLogoBadge
                    logoId={logoIdForStoragePreset(d.preset)}
                    className="size-4 p-px"
                  />
                  {STORAGE_PRESET_LABEL[d.preset]}
                </Badge>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {d.bucket}
              </TableCell>
              <TableCell className="text-muted-foreground">
                <div className="flex flex-col gap-0.5">
                  {d.region ? <span>{d.region}</span> : null}
                  {d.endpoint ? (
                    <span className="text-xs break-all">{d.endpoint}</span>
                  ) : null}
                  {!d.region && !d.endpoint ? '-' : null}
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {d.archive_policies}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-2">
                  <TestButton dest={d} />
                  <DeleteButton dest={d} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
