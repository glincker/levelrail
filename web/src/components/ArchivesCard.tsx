import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArchiveGlassIcon,
  BadgeCheckGlassIcon,
  DownloadGlassIcon,
  FileSearchGlassIcon,
  RotateCcwGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { toast } from '@/components/ui/toast'
import { formatAge, formatBytes } from '../lib/format'
import {
  archiveDownloadUrl,
  useArchives,
  useDeleteArchive,
  useRestoreArchive,
  useRunArchive,
  useVerifyArchive,
} from '../queries/archives'
import type { ArchiveBundle } from '../queries/archives'
import { ArchiveBrowseDialog } from './ArchiveBrowseDialog'
import { ArchivePolicyForm } from './ArchivePolicyForm'

function VerifiedLabel({ bundle }: { bundle: ArchiveBundle }) {
  const { t } = useTranslation('archives')
  if (bundle.verified_at === undefined) {
    return <span>{t('neverVerified')}</span>
  }
  const age = formatAge(bundle.verified_at)
  return bundle.verified_ok === true ? (
    <span>{t('verified', { age })}</span>
  ) : (
    <span className="text-destructive">{t('verifyFailed', { age })}</span>
  )
}

function BundleRow({
  bundle,
  onBrowse,
  onRestore,
  onDelete,
}: {
  bundle: ArchiveBundle
  onBrowse: (b: ArchiveBundle) => void
  onRestore: (b: ArchiveBundle) => void
  onDelete: (b: ArchiveBundle) => void
}) {
  const { t } = useTranslation('archives')
  const verify = useVerifyArchive()
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 py-2">
      <div className="min-w-0">
        <p className="truncate font-mono text-sm text-foreground">
          {bundle.id}
        </p>
        <p className="text-xs text-muted-foreground">
          {t(`kind.${bundle.kind}`)}
          {' · '}
          {t('bundleMeta', {
            rows: bundle.total_rows,
            size: formatBytes(bundle.size_bytes),
            age: formatAge(bundle.created_at),
          })}
          {' · '}
          <VerifiedLabel bundle={bundle} />
          {bundle.restored_at ? (
            <>
              {' · '}
              {t('restored', { age: formatAge(bundle.restored_at) })}
            </>
          ) : null}
        </p>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            onBrowse(bundle)
          }}
        >
          <FileSearchGlassIcon className="size-4" aria-hidden="true" />
          {t('browse')}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={verify.isPending}
          onClick={() => {
            verify.mutate(bundle.id, {
              onSuccess: (r) => {
                toast.add(
                  r.ok
                    ? { title: t('verifyOk'), type: 'success' }
                    : {
                        title: t('verifyFail', { error: r.error ?? '' }),
                        type: 'error',
                      },
                )
              },
              onError: (e) => {
                toast.add({ title: e.message, type: 'error' })
              },
            })
          }}
        >
          <BadgeCheckGlassIcon
            className="size-4 text-tone-success"
            aria-hidden="true"
          />
          {verify.isPending ? t('verifying') : t('verify')}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            onRestore(bundle)
          }}
        >
          <RotateCcwGlassIcon className="size-4" aria-hidden="true" />
          {t('restore')}
        </Button>
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          render={<a href={archiveDownloadUrl(bundle.id)} download />}
        >
          <DownloadGlassIcon className="size-4" aria-hidden="true" />
          {t('download')}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            onDelete(bundle)
          }}
        >
          <TrashGlassIcon className="size-4" aria-hidden="true" />
          {t('delete')}
        </Button>
      </div>
    </div>
  )
}

function BundleList({
  bundles,
  onBrowse,
  onRestore,
  onDelete,
}: {
  bundles: ArchiveBundle[]
  onBrowse: (b: ArchiveBundle) => void
  onRestore: (b: ArchiveBundle) => void
  onDelete: (b: ArchiveBundle) => void
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: bundles.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 76,
    overscan: 6,
  })
  return (
    <div ref={parentRef} className="max-h-96 overflow-auto">
      <div
        className="relative divide-y divide-border"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((v) => {
          const bundle = bundles[v.index]
          if (!bundle) return null
          return (
            <div
              key={v.key}
              data-index={v.index}
              ref={virtualizer.measureElement}
              className="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${v.start}px)` }}
            >
              <BundleRow
                bundle={bundle}
                onBrowse={onBrowse}
                onRestore={onRestore}
                onDelete={onDelete}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function ArchivesCard() {
  const { t } = useTranslation('archives')
  const { data, isLoading, isError, refetch } = useArchives()
  const run = useRunArchive()
  const restore = useRestoreArchive()
  const del = useDeleteArchive()
  const [browsing, setBrowsing] = useState<ArchiveBundle | null>(null)
  const [restoring, setRestoring] = useState<ArchiveBundle | null>(null)
  const [deleting, setDeleting] = useState<ArchiveBundle | null>(null)

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <ArchiveGlassIcon className="size-4.5" />
            </div>
            <div>
              <CardTitle>{t('title')}</CardTitle>
              <CardDescription>{t('description')}</CardDescription>
            </div>
          </div>
          <Button
            size="sm"
            disabled={run.isPending}
            onClick={() => {
              run.mutate(undefined, {
                onSuccess: (r) => {
                  const rows = r.created.reduce((n, b) => n + b.total_rows, 0)
                  toast.add(
                    r.created.length === 0
                      ? { title: t('nothingToArchive'), type: 'info' }
                      : {
                          title: t('archivedToast', {
                            count: rows,
                            bundles: r.created.length,
                          }),
                          type: 'success',
                        },
                  )
                },
                onError: (e) => {
                  toast.add({ title: e.message, type: 'error' })
                },
              })
            }}
          >
            {run.isPending ? t('archiving') : t('archiveNow')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        {isLoading ? <ListSkeleton /> : null}
        {isError ? (
          <Alert variant="destructive">
            <AlertDescription className="flex items-center justify-between gap-3">
              {t('loadError')}
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void refetch()
                }}
              >
                {t('retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {data ? (
          <>
            <p className="text-xs text-muted-foreground">
              {(['audit', 'deploys', 'pipelines'] as const)
                .map(
                  (k) =>
                    `${t(`kind.${k}`)}: ${t('hotRows', { count: data.hot_rows[k] ?? 0 })}`,
                )
                .join(' · ')}
            </p>
            {data.bundles.length === 0 ? (
              <EmptyState
                compact
                icon={<ArchiveGlassIcon className="size-5.5" />}
                title={t('emptyTitle')}
                description={t('empty')}
              />
            ) : (
              <BundleList
                bundles={data.bundles}
                onBrowse={setBrowsing}
                onRestore={setRestoring}
                onDelete={setDeleting}
              />
            )}
            <ArchivePolicyForm policy={data.policy} />
            <p className="text-xs text-muted-foreground">
              {t('dump.description')}
            </p>
          </>
        ) : null}
      </CardContent>
      <ArchiveBrowseDialog
        bundle={browsing}
        onClose={() => {
          setBrowsing(null)
        }}
      />
      <ConfirmDialog
        open={restoring !== null}
        onOpenChange={(open) => {
          if (!open) {
            restore.reset()
            setRestoring(null)
          }
        }}
        title={t('restoreTitle')}
        description={t('restoreDescription')}
        confirmLabel={t('restore')}
        pendingLabel={t('restoring')}
        pending={restore.isPending}
        error={restore.isError ? restore.error.message : null}
        onConfirm={() => {
          if (restoring === null) return
          restore.mutate(restoring.id, {
            onSuccess: (r) => {
              const n = Object.values(r.restored).reduce((a, b) => a + b, 0)
              toast.add({
                title: t('restoreToast', { count: n }),
                type: 'success',
              })
              setRestoring(null)
            },
          })
        }}
      />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) {
            del.reset()
            setDeleting(null)
          }
        }}
        title={t('deleteTitle')}
        description={t('deleteDescription')}
        confirmLabel={t('delete')}
        pendingLabel={t('deleting')}
        tone="destructive"
        pending={del.isPending}
        error={del.isError ? del.error.message : null}
        onConfirm={() => {
          if (deleting === null) return
          del.mutate(deleting.id, {
            onSuccess: () => {
              toast.add({ title: t('deleteToast'), type: 'success' })
              setDeleting(null)
            },
          })
        }}
      />
    </Card>
  )
}
