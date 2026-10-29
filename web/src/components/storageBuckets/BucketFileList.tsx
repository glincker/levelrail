import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  DownloadGlassIcon,
  LinkGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '@/lib/modelPreflight'
import {
  bucketDownloadUrl,
  useDeleteBucketObject,
  usePresignBucketObject,
} from '../../queries/storageBuckets'
import type { BucketObject } from '../../types/storageBuckets'

const GRID =
  'grid grid-cols-[minmax(0,1fr)_5.5rem_9rem_7rem] items-center gap-3 px-3'
const ROW_HEIGHT = 40
const LINK_SECONDS = 900

interface Props {
  bucketId: string
  objects: BucketObject[]
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
}

/** BucketFileList is the virtualized file table of one bucket. */
export function BucketFileList({
  bucketId,
  objects,
  hasMore,
  loadingMore,
  onLoadMore,
}: Readonly<Props>) {
  const { t } = useTranslation('storageBuckets')
  const parentRef = useRef<HTMLDivElement>(null)
  const [toDelete, setToDelete] = useState<string | null>(null)
  const remove = useDeleteBucketObject()
  const presign = usePresignBucketObject()
  const virtualizer = useVirtualizer({
    count: objects.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })

  function copyLink(key: string) {
    presign.mutate(
      { id: bucketId, key, method: 'GET', expiresSeconds: LINK_SECONDS },
      {
        onSuccess: (res) => {
          void navigator.clipboard
            .writeText(res.url)
            .then(() =>
              toast.add({ title: t('browser.linkCopied'), type: 'success' }),
            )
        },
        onError: (error) =>
          toast.add({
            title: t('browser.linkFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function confirmDelete() {
    if (!toDelete) return
    remove.mutate(
      { id: bucketId, key: toDelete },
      {
        onSuccess: () => {
          setToDelete(null)
          toast.add({ title: t('browser.deleted'), type: 'success' })
        },
        onError: (error) =>
          toast.add({
            title: t('browser.deleteFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <div className="space-y-2">
      <div className="rounded-lg border border-border bg-card">
        <div
          className={`${GRID} border-b border-border py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
        >
          <span>{t('browser.colName')}</span>
          <span>{t('browser.colSize')}</span>
          <span>{t('browser.colModified')}</span>
          <span className="text-right">{t('browser.colActions')}</span>
        </div>
        <div ref={parentRef} className="h-72 overflow-auto">
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((v) => {
              const o = objects[v.index]
              if (!o) return null
              return (
                <div
                  key={v.key}
                  className={`${GRID} absolute top-0 left-0 w-full border-b border-border/50 text-sm`}
                  style={{
                    height: v.size,
                    transform: `translateY(${v.start}px)`,
                  }}
                >
                  <span className="truncate font-mono text-xs" title={o.key}>
                    {o.key}
                  </span>
                  <span className="text-muted-foreground tabular-nums">
                    {formatBytes(o.size)}
                  </span>
                  <span className="text-muted-foreground">
                    {new Date(o.last_modified).toLocaleDateString()}
                  </span>
                  <span className="flex justify-end gap-1">
                    <Button
                      render={
                        <a
                          href={bucketDownloadUrl(bucketId, o.key)}
                          download
                          aria-label={t('browser.download')}
                        />
                      }
                      nativeButton={false}
                      variant="ghost"
                      size="icon-xs"
                    >
                      <DownloadGlassIcon size="1.2em" aria-hidden="true" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      aria-label={t('browser.copyLink')}
                      disabled={presign.isPending}
                      onClick={() => copyLink(o.key)}
                    >
                      <LinkGlassIcon size="1.2em" aria-hidden="true" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      aria-label={t('browser.delete')}
                      onClick={() => setToDelete(o.key)}
                    >
                      <TrashGlassIcon size="1.2em" aria-hidden="true" />
                    </Button>
                  </span>
                </div>
              )
            })}
          </div>
        </div>
      </div>
      {hasMore ? (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={loadingMore}
            onClick={onLoadMore}
          >
            {t('browser.loadMore')}
          </Button>
        </div>
      ) : null}
      <ConfirmDialog
        open={toDelete !== null}
        onOpenChange={(open) => {
          if (!open) setToDelete(null)
        }}
        title={t('browser.deleteTitle')}
        description={t('browser.deleteDescription', { key: toDelete ?? '' })}
        confirmLabel={t('browser.delete')}
        tone="destructive"
        pending={remove.isPending}
        onConfirm={confirmDelete}
      />
    </div>
  )
}
