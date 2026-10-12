import { useState, type ReactNode } from 'react'
import {
  CloudArrowUpIcon,
  PlugsConnectedIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
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
import { BrandTile } from './BrandLogoBadge'
import { STORAGE_PRESET_LABEL } from '../lib/storageProviders'
import { storagePresetBrand } from '../lib/providerBrands'
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
  const { t } = useTranslation('settings')
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
  const columns: ResponsiveColumn<StorageDestination>[] = [
    {
      id: 'name',
      primary: true,
      header: 'Name',
      cell: (d) => (
        <span className="font-medium text-foreground">{d.name}</span>
      ),
    },
    {
      id: 'provider',
      header: 'Provider',
      priority: 2,
      cell: (d) => (
        <Badge variant="outline" className="gap-1.5">
          <BrandTile
            name={storagePresetBrand(d.preset)}
            label={STORAGE_PRESET_LABEL[d.preset]}
            size="xs"
          />
          {STORAGE_PRESET_LABEL[d.preset]}
        </Badge>
      ),
    },
    {
      id: 'bucket',
      header: 'Bucket',
      cell: (d) => <span className="text-muted-foreground">{d.bucket}</span>,
    },
    {
      id: 'location',
      header: 'Region / endpoint',
      priority: 3,
      cell: (d) => (
        <div className="flex flex-col gap-0.5 text-muted-foreground">
          {d.region ? <span>{d.region}</span> : null}
          {d.endpoint ? (
            <span className="text-xs break-all">{d.endpoint}</span>
          ) : null}
          {!d.region && !d.endpoint ? '-' : null}
        </div>
      ),
    },
    {
      id: 'policies',
      header: 'Archive policies',
      priority: 3,
      cell: (d) => (
        <span className="text-muted-foreground">{d.archive_policies}</span>
      ),
    },
  ]

  return (
    <ResponsiveTable
      rows={destinations}
      columns={columns}
      getRowId={(d) => d.id}
      ariaLabel={t('tables.storageDestinations')}
      renderActions={(d) => (
        <div className="flex flex-wrap items-center justify-end gap-2">
          <TestButton dest={d} />
          <DeleteButton dest={d} />
        </div>
      )}
    />
  )
}
