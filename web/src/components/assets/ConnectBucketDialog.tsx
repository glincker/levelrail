import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
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
import { useStorageDestinationsOptional } from '../../queries/storage'
import { useCreateAssetBucket } from '../../queries/assets'
import type { AssetServingMode } from '../../types/assets'

const NAME_RE = /^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$/

export function ConnectBucketDialog({
  onCreated,
}: Readonly<{ onCreated?: (id: string) => void }>) {
  const { t } = useTranslation('assets')
  const [open, setOpen] = useState(false)
  const destinations = (useStorageDestinationsOptional().data ?? []).filter(
    (d) => d.kind === 's3',
  )
  const create = useCreateAssetBucket()
  const [name, setName] = useState('')
  const [targetId, setTargetId] = useState('')
  const [prefix, setPrefix] = useState('')
  const [isPublic, setIsPublic] = useState(false)
  const [mode, setMode] = useState<AssetServingMode>('direct')
  const [publicBaseUrl, setPublicBaseUrl] = useState('')
  const [cdnBaseUrl, setCdnBaseUrl] = useState('')
  const [confirmName, setConfirmName] = useState('')
  const effectiveTarget = targetId || destinations[0]?.id || ''

  function reset() {
    setName('')
    setTargetId('')
    setPrefix('')
    setIsPublic(false)
    setMode('direct')
    setPublicBaseUrl('')
    setCdnBaseUrl('')
    setConfirmName('')
    create.reset()
  }

  function onOpenChange(next: boolean) {
    setOpen(next)
    if (!next) reset()
  }

  const nameValid = NAME_RE.test(name)
  const cdnMissing = mode === 'cdn' && cdnBaseUrl.trim() === ''
  const confirmOk = !isPublic || confirmName === name
  const canSubmit =
    nameValid && effectiveTarget !== '' && !cdnMissing && confirmOk

  function submit() {
    create.mutate(
      {
        name,
        target_id: effectiveTarget,
        prefix: prefix.trim() || undefined,
        visibility: isPublic ? 'public' : 'private',
        serving_mode: mode,
        public_base_url: publicBaseUrl.trim() || undefined,
        cdn_base_url: mode === 'cdn' ? cdnBaseUrl.trim() : undefined,
        confirm_name: isPublic ? confirmName : undefined,
      },
      {
        onSuccess: (bucket) => {
          toast.add({ title: t('connect.created'), type: 'success' })
          onOpenChange(false)
          onCreated?.(bucket.id)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger render={<Button />}>
        <PlusGlassIcon size="1.2em" aria-hidden="true" />
        {t('connect.button')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('connect.title')}</DialogTitle>
          <DialogDescription>{t('connect.description')}</DialogDescription>
        </DialogHeader>
        {destinations.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('connect.noDestination')}{' '}
            <Link to="/settings/storage" className="underline">
              {t('connect.addDestination')}
            </Link>
          </p>
        ) : (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="asset-destination">
                {t('connect.destination')}
              </Label>
              <Select
                value={effectiveTarget}
                onValueChange={(v) => {
                  if (v) setTargetId(v)
                }}
              >
                <SelectTrigger id="asset-destination" className="w-full">
                  <SelectValue
                    placeholder={t('connect.destinationPlaceholder')}
                  />
                </SelectTrigger>
                <SelectContent>
                  {destinations.map((d) => (
                    <SelectItem key={d.id} value={d.id}>
                      {d.name} ({d.bucket})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="asset-name">{t('connect.name')}</Label>
                <Input
                  id="asset-name"
                  value={name}
                  aria-invalid={name !== '' && !nameValid}
                  onChange={(e) => {
                    setName(e.target.value)
                  }}
                />
                <p className="text-xs text-muted-foreground">
                  {t('connect.nameHint')}
                </p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="asset-prefix">{t('connect.prefix')}</Label>
                <Input
                  id="asset-prefix"
                  value={prefix}
                  placeholder="uploads/"
                  onChange={(e) => {
                    setPrefix(e.target.value)
                  }}
                />
                <p className="text-xs text-muted-foreground">
                  {t('connect.prefixHint')}
                </p>
              </div>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="asset-serving">{t('connect.serving')}</Label>
                <Select
                  value={mode}
                  onValueChange={(v) => {
                    if (v === 'direct' || v === 'cdn') setMode(v)
                  }}
                >
                  <SelectTrigger id="asset-serving" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="direct">
                      {t('connect.direct')}
                    </SelectItem>
                    <SelectItem value="cdn">{t('connect.cdn')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-end justify-between gap-3 pb-1">
                <Label htmlFor="asset-public">{t('connect.public')}</Label>
                <Switch
                  id="asset-public"
                  checked={isPublic}
                  onCheckedChange={setIsPublic}
                />
              </div>
            </div>
            {mode === 'cdn' ? (
              <div className="space-y-1.5">
                <Label htmlFor="asset-cdn">{t('connect.cdnBaseUrl')}</Label>
                <Input
                  id="asset-cdn"
                  value={cdnBaseUrl}
                  placeholder="https://cdn.example.com"
                  onChange={(e) => {
                    setCdnBaseUrl(e.target.value)
                  }}
                />
                <p className="text-xs text-muted-foreground">
                  {t('connect.cdnBaseUrlHint')}
                </p>
              </div>
            ) : null}
            {isPublic ? (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="asset-public-url">
                    {t('connect.publicBaseUrl')}
                  </Label>
                  <Input
                    id="asset-public-url"
                    value={publicBaseUrl}
                    placeholder="https://assets.example.com"
                    onChange={(e) => {
                      setPublicBaseUrl(e.target.value)
                    }}
                  />
                  <p className="text-xs text-muted-foreground">
                    {t('connect.publicBaseUrlHint')}
                  </p>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="asset-confirm">
                    {t('connect.confirmName')}
                  </Label>
                  <Input
                    id="asset-confirm"
                    value={confirmName}
                    autoComplete="off"
                    onChange={(e) => {
                      setConfirmName(e.target.value)
                    }}
                  />
                </div>
              </>
            ) : null}
            {create.isError ? (
              <p role="alert" className="text-sm text-destructive">
                {t('connect.failed')}: {create.error.message}
              </p>
            ) : null}
          </div>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('connect.cancel')}
          </Button>
          <Button
            type="button"
            disabled={!canSubmit || create.isPending}
            onClick={submit}
          >
            {create.isPending ? t('connect.submitting') : t('connect.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
