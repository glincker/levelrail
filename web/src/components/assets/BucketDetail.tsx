import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftGlassIcon, PlugGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { useAssetBucket, useDeleteAssetBucket } from '../../queries/assets'
import { PresetLabel, ServingBadge, VisibilityBadge } from './AssetBadges'
import { AccessCard } from './AccessCard'
import { AssetList } from './AssetList'
import { ProbePanel } from './ProbePanel'
import { UploadZone } from './UploadZone'
import { UsageCard } from './UsageCard'

export function BucketDetail({
  id,
  onBack,
}: Readonly<{ id: string; onBack: () => void }>) {
  const { t } = useTranslation('assets')
  const { data: bucket, isPending, isError } = useAssetBucket(id)
  const remove = useDeleteAssetBucket()
  const [confirmOpen, setConfirmOpen] = useState(false)

  if (isPending) return <Skeleton className="h-40 w-full" />
  if (isError) {
    return (
      <div className="space-y-2">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeftGlassIcon size="1.2em" aria-hidden="true" />
          {t('back')}
        </Button>
        <p className="text-sm text-destructive">{t('detail.loadFailed')}</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeftGlassIcon size="1.2em" aria-hidden="true" />
          {t('back')}
        </Button>
        <h2 className="text-base font-semibold">{bucket.name}</h2>
        <VisibilityBadge visibility={bucket.visibility} />
        <ServingBadge mode={bucket.serving_mode} />
        <span className="text-xs text-muted-foreground">
          <PresetLabel preset={bucket.preset} /> {'·'} {t('detail.prefix')}:{' '}
          <code>{bucket.prefix || t('detail.noPrefix')}</code>
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="ml-auto"
          onClick={() => {
            remove.reset()
            setConfirmOpen(true)
          }}
        >
          <PlugGlassIcon size="1.2em" aria-hidden="true" />
          {t('detail.disconnect')}
        </Button>
      </div>
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_21rem]">
        <div className="min-w-0 space-y-3">
          <UploadZone bucketId={bucket.id} />
          <AssetList bucket={bucket} />
        </div>
        <div className="space-y-3">
          <ProbePanel bucketId={bucket.id} />
          <AccessCard bucket={bucket} />
          <UsageCard bucketId={bucket.id} />
        </div>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('detail.disconnectTitle', { name: bucket.name })}
        description={t('detail.disconnectBody')}
        consequences={[t('detail.disconnectKeeps')]}
        tone="destructive"
        confirmLabel={t('detail.disconnectConfirm')}
        pendingLabel={t('detail.disconnecting')}
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() => {
          remove.mutate(bucket.id, {
            onSuccess: () => {
              setConfirmOpen(false)
              onBack()
            },
          })
        }}
      />
    </div>
  )
}
