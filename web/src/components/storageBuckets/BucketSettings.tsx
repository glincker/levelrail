import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useRunBucketLifecycle,
  useUpdateStorageBucket,
} from '../../queries/storageBuckets'
import type { StorageBucket } from '../../types/storageBuckets'

const BYTES_PER_MB = 1024 * 1024

function parseNonNegative(value: string): number {
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) && n > 0 ? n : 0
}

/** BucketSettings edits the public flag, size limit and expiry rule of one bucket. */
export function BucketSettings({
  bucket,
}: Readonly<{ bucket: StorageBucket }>) {
  const { t } = useTranslation('storageBuckets')
  const [isPublic, setIsPublic] = useState(bucket.public)
  const [quotaMb, setQuotaMb] = useState(
    String(Math.round(bucket.quota_bytes / BYTES_PER_MB)),
  )
  const [days, setDays] = useState(String(bucket.lifecycle_days))
  const update = useUpdateStorageBucket()
  const sweep = useRunBucketLifecycle()

  function save() {
    update.mutate(
      {
        id: bucket.id,
        req: {
          public: isPublic,
          quota_bytes: parseNonNegative(quotaMb) * BYTES_PER_MB,
          lifecycle_days: parseNonNegative(days),
        },
      },
      {
        onSuccess: () =>
          toast.add({ title: t('settings.saved'), type: 'success' }),
        onError: (error) =>
          toast.add({
            title: t('settings.failed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function runSweep() {
    sweep.mutate(bucket.id, {
      onSuccess: (res) =>
        toast.add({
          title: t('settings.swept', { count: res.deleted }),
          type: 'success',
        }),
      onError: (error) =>
        toast.add({
          title: t('settings.sweepFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <p className="text-sm font-medium text-foreground">
        {t('settings.title')}
      </p>
      <div className="flex items-start justify-between gap-4">
        <div>
          <Label htmlFor={`bucket-public-${bucket.id}`}>
            {t('settings.public')}
          </Label>
          <p className="text-sm text-muted-foreground">
            {t('settings.publicHint')}
          </p>
        </div>
        <Switch
          id={`bucket-public-${bucket.id}`}
          checked={isPublic}
          onCheckedChange={setIsPublic}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={`bucket-quota-${bucket.id}`}>
            {t('settings.quota')}
          </Label>
          <Input
            id={`bucket-quota-${bucket.id}`}
            inputMode="numeric"
            value={quotaMb}
            onChange={(e) => setQuotaMb(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`bucket-expire-${bucket.id}`}>
            {t('settings.expire')}
          </Label>
          <Input
            id={`bucket-expire-${bucket.id}`}
            inputMode="numeric"
            value={days}
            onChange={(e) => setDays(e.target.value)}
          />
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          disabled={update.isPending}
          onClick={save}
        >
          {update.isPending ? t('settings.saving') : t('settings.save')}
        </Button>
        {bucket.lifecycle_days > 0 ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={sweep.isPending}
            onClick={runSweep}
          >
            {sweep.isPending ? t('settings.sweeping') : t('settings.sweep')}
          </Button>
        ) : null}
        {bucket.last_swept_at ? (
          <span className="text-xs text-muted-foreground">
            {t('settings.lastSwept', {
              when: new Date(bucket.last_swept_at).toLocaleString(),
            })}
          </span>
        ) : null}
      </div>
    </div>
  )
}
