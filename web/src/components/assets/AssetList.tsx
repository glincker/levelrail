import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CopyGlassIcon,
  FileGlassIcon,
  GlobeGlassIcon,
  LinkGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  assetUrlsQueryOptions,
  useAssetList,
  useDeleteAsset,
} from '../../queries/assets'
import {
  baseName,
  copySnippet,
  formatAssetSize,
  isImageAsset,
  type CopyFormat,
} from '../../lib/assetsFormat'
import type { AssetBucket, AssetItem } from '../../types/assets'

type UrlKind = 'origin' | 'cdn'

function Thumb({
  bucketId,
  item,
}: Readonly<{ bucketId: string; item: AssetItem }>) {
  const { t } = useTranslation('assets')
  const image = isImageAsset(item.key, item.content_type)
  const { data } = useQuery({
    ...assetUrlsQueryOptions(bucketId, item.key),
    enabled: image,
  })
  const src = data?.origin_url ?? data?.cdn_url ?? data?.presigned_url
  return (
    <div className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
      {image && src ? (
        <img
          src={src}
          alt={t('items.thumbAlt', { name: baseName(item.key) })}
          loading="lazy"
          className="size-full object-cover"
        />
      ) : (
        <FileGlassIcon
          aria-hidden="true"
          className="size-6 text-muted-foreground"
        />
      )}
    </div>
  )
}

function AssetRow({
  bucket,
  item,
  format,
  onDelete,
}: Readonly<{
  bucket: AssetBucket
  item: AssetItem
  format: CopyFormat
  onDelete: (item: AssetItem) => void
}>) {
  const { t } = useTranslation('assets')
  const queryClient = useQueryClient()
  const name = baseName(item.key)
  const isPublic = bucket.visibility === 'public'
  const hasCdn = Boolean(bucket.cdn_base_url)

  async function copy(kind: UrlKind) {
    try {
      const urls = await queryClient.fetchQuery(
        assetUrlsQueryOptions(bucket.id, item.key),
      )
      const url =
        kind === 'cdn' ? urls.cdn_url : (urls.origin_url ?? urls.presigned_url)
      if (!url) {
        toast.add({ title: t('items.copyFailed'), type: 'error' })
        return
      }
      await navigator.clipboard.writeText(copySnippet(format, url, name))
      toast.add({ title: t('items.copied'), type: 'success' })
    } catch (err) {
      toast.add({
        title: t('items.copyFailed'),
        description: err instanceof Error ? err.message : undefined,
        type: 'error',
      })
    }
  }

  return (
    <li className="flex items-center gap-2.5 rounded-lg border border-border bg-card px-2.5 py-2">
      <Thumb bucketId={bucket.id} item={item} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium" title={item.key}>
          {item.key}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {formatAssetSize(item.size)}
          {item.content_type ? ` · ${item.content_type}` : ''}
        </p>
      </div>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t(isPublic ? 'items.copyOrigin' : 'items.copySigned', {
          name,
        })}
        title={t(isPublic ? 'items.origin' : 'items.signed')}
        onClick={() => {
          void copy('origin')
        }}
      >
        {isPublic ? (
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        ) : (
          <LinkGlassIcon size="1.4em" aria-hidden="true" />
        )}
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        disabled={!hasCdn || !isPublic}
        aria-label={t('items.copyCdn', { name })}
        title={hasCdn ? t('items.cdn') : t('items.noCdn')}
        onClick={() => {
          void copy('cdn')
        }}
      >
        <GlobeGlassIcon size="1.4em" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('items.delete', { name })}
        onClick={() => {
          onDelete(item)
        }}
      >
        <TrashGlassIcon size="1.4em" aria-hidden="true" />
      </Button>
    </li>
  )
}

export function AssetList({ bucket }: Readonly<{ bucket: AssetBucket }>) {
  const { t } = useTranslation('assets')
  const { data, isPending, isError } = useAssetList(bucket.id)
  const del = useDeleteAsset(bucket.id)
  const [format, setFormat] = useState<CopyFormat>('url')
  const [target, setTarget] = useState<AssetItem | null>(null)

  const formats: { id: CopyFormat; label: string }[] = [
    { id: 'url', label: t('items.formatUrl') },
    { id: 'markdown', label: t('items.formatMarkdown') },
    { id: 'img', label: t('items.formatImg') },
  ]

  return (
    <section aria-labelledby="asset-files-title" className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="asset-files-title" className="text-sm font-medium">
          {t('items.title')}
        </h2>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span id="asset-copy-as">{t('items.copyAs')}</span>
          <Select
            value={format}
            onValueChange={(v) => {
              const found = formats.find((f) => f.id === v)
              if (found) setFormat(found.id)
            }}
          >
            <SelectTrigger
              size="sm"
              className="w-28"
              aria-labelledby="asset-copy-as"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {formats.map((f) => (
                <SelectItem key={f.id} value={f.id}>
                  {f.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      {bucket.visibility === 'private' ? (
        <p className="text-xs text-muted-foreground">{t('items.signedNote')}</p>
      ) : null}
      {isPending ? (
        <Skeleton className="h-14 w-full" />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('items.loadFailed')}</p>
      ) : data.assets.length === 0 ? (
        <EmptyState
          compact
          className="gap-2 border-0 bg-transparent py-4"
          icon={<FileGlassIcon className="size-6" />}
          title={t('items.title')}
          description={t('items.empty')}
        />
      ) : (
        <ul className="grid gap-2 xl:grid-cols-2">
          {data.assets.map((item) => (
            <AssetRow
              key={item.key}
              bucket={bucket}
              item={item}
              format={format}
              onDelete={setTarget}
            />
          ))}
        </ul>
      )}
      {data?.cursor ? (
        <p className="text-xs text-muted-foreground">{t('items.more')}</p>
      ) : null}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('items.deleteTitle', { name: target?.key ?? '' })}
        description={t('items.deleteBody')}
        tone="destructive"
        confirmLabel={t('items.deleteConfirm')}
        pendingLabel={t('items.deleting')}
        pending={del.isPending}
        error={del.isError ? del.error.message : null}
        onConfirm={() => {
          if (!target) return
          del.mutate(target.key, {
            onSuccess: () => {
              setTarget(null)
            },
          })
        }}
      />
    </section>
  )
}
