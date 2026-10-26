import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useUpdateAssetBucket } from '../../queries/assets'
import type { AssetBucket, AssetServingMode } from '../../types/assets'

function VisibilityToggle({ bucket }: Readonly<{ bucket: AssetBucket }>) {
  const { t } = useTranslation('assets')
  const update = useUpdateAssetBucket(bucket.id)
  const [open, setOpen] = useState(false)
  const makingPublic = bucket.visibility === 'private'

  return (
    <div className="flex items-center justify-between gap-3">
      <Label htmlFor="asset-visibility">{t('access.visibility')}</Label>
      <div className="flex items-center gap-2 text-xs">
        <span>{t(`visibility.${bucket.visibility}`)}</span>
        <Switch
          id="asset-visibility"
          checked={bucket.visibility === 'public'}
          onCheckedChange={() => {
            update.reset()
            setOpen(true)
          }}
        />
      </div>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t(makingPublic ? 'access.publicTitle' : 'access.privateTitle', {
          name: bucket.name,
        })}
        description={t(
          makingPublic ? 'access.publicBody' : 'access.privateBody',
        )}
        consequences={
          makingPublic
            ? [
                t('access.publicRisk1'),
                t('access.publicRisk2'),
                t('access.publicRisk3'),
                t('access.publicRisk4'),
              ]
            : undefined
        }
        tone={makingPublic ? 'destructive' : 'default'}
        requireTyped={makingPublic ? bucket.name : undefined}
        confirmLabel={t(
          makingPublic ? 'access.confirmPublic' : 'access.confirmPrivate',
        )}
        pendingLabel={t('access.saving')}
        pending={update.isPending}
        error={update.isError ? update.error.message : null}
        onConfirm={() => {
          update.mutate(
            makingPublic
              ? { visibility: 'public', confirm_name: bucket.name }
              : { visibility: 'private' },
            {
              onSuccess: () => {
                setOpen(false)
              },
            },
          )
        }}
      />
    </div>
  )
}

function ServingForm({ bucket }: Readonly<{ bucket: AssetBucket }>) {
  const { t } = useTranslation('assets')
  const update = useUpdateAssetBucket(bucket.id)
  const [mode, setMode] = useState<AssetServingMode>(
    bucket.serving_mode === 'cdn' ? 'cdn' : 'direct',
  )
  const [publicUrl, setPublicUrl] = useState(bucket.public_base_url ?? '')
  const [cdnUrl, setCdnUrl] = useState(bucket.cdn_base_url ?? '')

  function save() {
    update.mutate(
      {
        serving_mode: mode,
        public_base_url: publicUrl.trim(),
        cdn_base_url: cdnUrl.trim(),
      },
      {
        onSuccess: () => {
          toast.add({ title: t('access.saved'), type: 'success' })
        },
      },
    )
  }

  return (
    <div className="space-y-2">
      <div className="grid grid-cols-[7rem_minmax(0,1fr)] items-center gap-2">
        <Label htmlFor="asset-mode">{t('access.serving')}</Label>
        <Select
          value={mode}
          onValueChange={(v) => {
            if (v === 'direct' || v === 'cdn') setMode(v)
          }}
        >
          <SelectTrigger id="asset-mode" size="sm" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="direct">{t('serving.direct')}</SelectItem>
            <SelectItem value="cdn">{t('serving.cdn')}</SelectItem>
          </SelectContent>
        </Select>
        <Label htmlFor="asset-pub-url">{t('access.publicBaseUrl')}</Label>
        <Input
          id="asset-pub-url"
          className="h-7"
          value={publicUrl}
          onChange={(e) => {
            setPublicUrl(e.target.value)
          }}
        />
        <Label htmlFor="asset-cdn-url">{t('access.cdnBaseUrl')}</Label>
        <Input
          id="asset-cdn-url"
          className="h-7"
          value={cdnUrl}
          onChange={(e) => {
            setCdnUrl(e.target.value)
          }}
        />
      </div>
      {update.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {t('access.saveFailed')}: {update.error.message}
        </p>
      ) : null}
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={update.isPending}
        onClick={save}
      >
        {update.isPending ? t('access.saving') : t('access.save')}
      </Button>
    </div>
  )
}

export function AccessCard({ bucket }: Readonly<{ bucket: AssetBucket }>) {
  const { t } = useTranslation('assets')
  return (
    <section
      aria-labelledby="asset-access-title"
      className="space-y-3 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="asset-access-title" className="text-sm font-medium">
        {t('access.title')}
      </h2>
      <VisibilityToggle bucket={bucket} />
      <ServingForm key={bucket.updated_at} bucket={bucket} />
    </section>
  )
}
