import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '@/lib/modelPreflight'
import {
  useBucketObjects,
  useBucketUsage,
  useUploadBucketObject,
} from '../../queries/storageBuckets'
import type { StorageBucket } from '../../types/storageBuckets'
import { BucketFileList } from './BucketFileList'
import { BucketSettings } from './BucketSettings'

/** BucketBrowserDialog lists, uploads and deletes the files of one bucket and edits its limits. */
export function BucketBrowserDialog({
  bucket,
  onClose,
}: Readonly<{ bucket: StorageBucket | null; onClose: () => void }>) {
  return (
    <Dialog
      open={bucket !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        {bucket ? <BrowserBody bucket={bucket} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function BrowserBody({ bucket }: Readonly<{ bucket: StorageBucket }>) {
  const { t } = useTranslation('storageBuckets')
  const [prefix, setPrefix] = useState('')
  const fileInput = useRef<HTMLInputElement>(null)
  const objects = useBucketObjects(bucket.id, prefix)
  const usage = useBucketUsage(bucket.id)
  const upload = useUploadBucketObject()
  const rows = objects.data?.pages.flatMap((p) => p.objects) ?? []

  function onPick(file: File | undefined) {
    if (!file) return
    const key = `${prefix.endsWith('/') ? prefix : ''}${file.name}`
    upload.mutate(
      { id: bucket.id, key, file },
      {
        onSuccess: () =>
          toast.add({ title: t('browser.uploaded', { key }), type: 'success' }),
        onError: (error) =>
          toast.add({
            title: t('browser.uploadFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
    if (fileInput.current) fileInput.current.value = ''
  }

  const used = usage.data?.bytes ?? 0
  const limit = bucket.quota_bytes
  const percent = limit > 0 ? Math.min(100, (used / limit) * 100) : 0

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('browser.title', { name: bucket.name })}</DialogTitle>
        <DialogDescription>{t('browser.description')}</DialogDescription>
      </DialogHeader>
      {usage.data ? (
        <div className="space-y-1">
          <p className="text-sm text-muted-foreground">
            {limit > 0
              ? t('browser.usage', {
                  used: formatBytes(used),
                  limit: formatBytes(limit),
                })
              : t('browser.usageNoLimit', { used: formatBytes(used) })}
            {' · '}
            {t('browser.files', { count: usage.data.objects })}
          </p>
          {limit > 0 ? <Progress value={percent} /> : null}
        </div>
      ) : null}
      {bucket.public && bucket.public_url ? (
        <Alert>
          <AlertDescription>
            {t('browser.publicLink')}:{' '}
            <code className="break-all font-mono text-xs">
              {bucket.public_url}
            </code>
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <Input
          className="max-w-xs"
          aria-label={t('browser.filter')}
          placeholder={t('browser.filter')}
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
        />
        <input
          ref={fileInput}
          type="file"
          className="sr-only"
          tabIndex={-1}
          aria-hidden="true"
          onChange={(e) => onPick(e.target.files?.[0])}
        />
        <Button
          type="button"
          size="sm"
          disabled={upload.isPending}
          onClick={() => fileInput.current?.click()}
        >
          <UploadGlassIcon size="1.2em" aria-hidden="true" />
          {upload.isPending ? t('browser.uploading') : t('browser.upload')}
        </Button>
      </div>
      {objects.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('browser.loadFailed')} {objects.error.message}
          </AlertDescription>
        </Alert>
      ) : rows.length === 0 && !objects.isPending ? (
        <EmptyState
          compact
          icon={<UploadGlassIcon className="size-5.5" aria-hidden="true" />}
          title={t('browser.empty')}
          description={t('browser.emptyHint')}
        />
      ) : (
        <BucketFileList
          bucketId={bucket.id}
          objects={rows}
          hasMore={objects.hasNextPage}
          loadingMore={objects.isFetchingNextPage}
          onLoadMore={() => void objects.fetchNextPage()}
        />
      )}
      <BucketSettings key={bucket.id} bucket={bucket} />
    </>
  )
}
