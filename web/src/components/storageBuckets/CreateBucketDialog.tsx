import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
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
import { useCreateStorageBucket } from '../../queries/storageBuckets'

const BYTES_PER_MB = 1024 * 1024
const DEFAULT_ENV_PREFIX = 'S3'

function toCount(value: string): number {
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) && n > 0 ? n : 0
}

/** CreateBucketDialog makes a bucket and binds it to the app in one step. */
export function CreateBucketDialog({
  appName,
  open,
  onOpenChange,
  onCreated,
}: Readonly<{
  appName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}>) {
  const { t } = useTranslation('storageBuckets')
  const destinations = useStorageDestinationsOptional().data ?? []
  const create = useCreateStorageBucket()
  const [name, setName] = useState(`${appName}-files`)
  const [targetId, setTargetId] = useState('')
  const [envPrefix, setEnvPrefix] = useState(DEFAULT_ENV_PREFIX)
  const [quotaMb, setQuotaMb] = useState('0')
  const [days, setDays] = useState('0')
  const [isPublic, setIsPublic] = useState(false)
  const effectiveTarget = targetId || destinations[0]?.id || ''

  function submit() {
    create.mutate(
      {
        name,
        target_id: effectiveTarget,
        app_name: appName,
        env_prefix: envPrefix,
        public: isPublic,
        quota_bytes: toCount(quotaMb) * BYTES_PER_MB,
        lifecycle_days: toCount(days),
      },
      {
        onSuccess: () => {
          onOpenChange(false)
          onCreated()
          toast.add({
            title: t('create.created'),
            description: t('create.createdBody', { app: appName }),
            type: 'success',
          })
        },
        onError: (error) =>
          toast.add({
            title: t('create.failed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        {destinations.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('create.noDestination')}{' '}
            <Link to="/settings/storage" className="underline">
              {t('create.connectDestination')}
            </Link>
          </p>
        ) : (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="bucket-name">{t('create.name')}</Label>
              <Input
                id="bucket-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="bucket-destination">
                {t('create.destination')}
              </Label>
              <Select
                value={effectiveTarget}
                onValueChange={(v) => {
                  if (v) setTargetId(v)
                }}
              >
                <SelectTrigger id="bucket-destination" className="w-full">
                  <SelectValue
                    placeholder={t('create.destinationPlaceholder')}
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
            <div className="space-y-1.5">
              <Label htmlFor="bucket-env-prefix">{t('create.envPrefix')}</Label>
              <Input
                id="bucket-env-prefix"
                value={envPrefix}
                onChange={(e) => setEnvPrefix(e.target.value.toUpperCase())}
              />
              <p className="text-sm text-muted-foreground">
                {t('create.envPrefixHint', {
                  prefix: envPrefix || DEFAULT_ENV_PREFIX,
                })}
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="bucket-new-quota">{t('create.quota')}</Label>
                <Input
                  id="bucket-new-quota"
                  inputMode="numeric"
                  value={quotaMb}
                  onChange={(e) => setQuotaMb(e.target.value)}
                />
                <p className="text-sm text-muted-foreground">
                  {t('create.quotaHint')}
                </p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="bucket-new-expire">{t('create.expire')}</Label>
                <Input
                  id="bucket-new-expire"
                  inputMode="numeric"
                  value={days}
                  onChange={(e) => setDays(e.target.value)}
                />
                <p className="text-sm text-muted-foreground">
                  {t('create.expireHint')}
                </p>
              </div>
            </div>
            <div className="flex items-start justify-between gap-4">
              <div>
                <Label htmlFor="bucket-new-public">{t('create.public')}</Label>
                <p className="text-sm text-muted-foreground">
                  {t('create.publicHint')}
                </p>
              </div>
              <Switch
                id="bucket-new-public"
                checked={isPublic}
                onCheckedChange={setIsPublic}
              />
            </div>
          </div>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('create.cancel')}
          </Button>
          <Button
            type="button"
            disabled={create.isPending || !effectiveTarget || name.length < 2}
            onClick={submit}
          >
            {create.isPending ? t('create.submitting') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
