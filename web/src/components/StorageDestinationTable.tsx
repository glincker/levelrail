import { useState, type ReactNode } from 'react'
import {
  PlugGlassIcon,
  ServerGlassIcon,
  TrashGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
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
import { StorageTestResultDialog } from './StorageTestResultDialog'
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
  const { t } = useTranslation('settings')
  const test = useTestStorageDestination()
  const [failed, setFailed] = useState<StorageProbeResult | null>(null)
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={test.isPending}
        onClick={() => {
          test.mutate(dest.id, {
            onSuccess: (result) => {
              if (result.ok) {
                toast.add({
                  title: t('storageDestinations.test.passed', {
                    name: dest.name,
                  }),
                  description: describeProbe(result),
                  type: 'success',
                })
                return
              }
              setFailed(result)
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
        <PlugGlassIcon className="size-4" aria-hidden="true" />
        {test.isPending ? 'Testing...' : 'Test'}
      </Button>
      {failed ? (
        <StorageTestResultDialog
          dest={dest}
          initial={failed}
          open
          onOpenChange={(next) => {
            if (!next) setFailed(null)
          }}
        />
      ) : null}
    </>
  )
}

function formatBytes(n: number): string {
  if (n < 1000) return `${n} B`
  const units = ['kB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = -1
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
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
        <TrashGlassIcon className="size-4" aria-hidden="true" />
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
        icon={<UploadGlassIcon className="size-5.5" />}
        title={t('storageDestinations.empty.title')}
        description={t('storageDestinations.empty.description')}
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
      cell: (d) =>
        d.preset === 'agent' ? (
          <Badge variant="outline" className="gap-1.5">
            <ServerGlassIcon className="size-4" aria-hidden="true" />
            {t('storageDestinations.kind.agent')}
          </Badge>
        ) : (
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
      cell: (d) => (
        <span className="text-muted-foreground">
          {d.kind === 'agent'
            ? t('storageDestinations.kind.agentLocation', {
                node: d.node_id,
                directory: d.directory,
              })
            : d.bucket}
        </span>
      ),
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
      id: 'usage',
      header: t('storageDestinations.usage.header'),
      priority: 2,
      cell: (d) => (
        <div className="flex flex-col gap-0.5">
          <span className="text-muted-foreground">
            {d.quota_bytes > 0
              ? t('storageDestinations.usage.ofQuota', {
                  used: formatBytes(d.used_bytes),
                  quota: formatBytes(d.quota_bytes),
                })
              : d.used_bytes > 0
                ? formatBytes(d.used_bytes)
                : t('storageDestinations.usage.none')}
          </span>
          {d.over_quota ? (
            <Badge variant="destructive" className="w-fit">
              {t('storageDestinations.usage.overQuota')}
            </Badge>
          ) : null}
        </div>
      ),
    },
    {
      id: 'cost',
      header: t('storageDestinations.usage.cost'),
      priority: 3,
      cell: (d) => (
        <span className="text-muted-foreground">
          {d.monthly_cost_usd > 0
            ? `$${d.monthly_cost_usd.toFixed(2)}`
            : t('storageDestinations.usage.free')}
        </span>
      ),
    },
    {
      id: 'defaults',
      header: t('storageDestinations.usage.defaultFor'),
      priority: 3,
      cell: (d) =>
        d.default_for.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {d.default_for.map((c) => (
              <Badge key={c} variant="muted">
                {t(`storageDestinations.classes.${c}`)}
              </Badge>
            ))}
          </div>
        ) : (
          <span className="text-muted-foreground">
            {t('storageDestinations.usage.noDefault')}
          </span>
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
