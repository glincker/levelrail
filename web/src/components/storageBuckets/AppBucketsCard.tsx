import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FolderGlassIcon,
  PlusGlassIcon,
  UnlinkGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useRestartApp } from '../../queries/apps'
import {
  useAllStorageBuckets,
  useBindAppBucket,
  useStorageBuckets,
  useUnbindAppBucket,
} from '../../queries/storageBuckets'
import type { AppDetail } from '../../types/appDetail'
import type { StorageBucket } from '../../types/storageBuckets'
import { BucketBrowserDialog } from './BucketBrowserDialog'
import { CreateBucketDialog } from './CreateBucketDialog'

const BYTES_PER_MB = 1024 * 1024

/** AppBucketsCard creates, binds and browses the storage buckets of one app. */
export function AppBucketsCard({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('storageBuckets')
  const buckets = useStorageBuckets(app.name)
  const all = useAllStorageBuckets()
  const bind = useBindAppBucket()
  const unbind = useUnbindAppBucket()
  const restart = useRestartApp()
  const [creating, setCreating] = useState(false)
  const [browsing, setBrowsing] = useState<StorageBucket | null>(null)
  const [unbinding, setUnbinding] = useState<StorageBucket | null>(null)
  const [pick, setPick] = useState('')
  const [needsRestart, setNeedsRestart] = useState(false)

  const bound = buckets.data ?? []
  const boundIds = new Set(bound.map((b) => b.id))
  const bindable = (all.data ?? []).filter((b) => !boundIds.has(b.id))

  function doBind() {
    if (!pick) return
    bind.mutate(
      { app: app.name, id: pick },
      {
        onSuccess: (res) => {
          setPick('')
          setNeedsRestart(!res.app_restarted)
          toast.add({
            title: t(res.app_restarted ? 'card.boundRestarted' : 'card.bound'),
            type: 'success',
          })
        },
        onError: (error) =>
          toast.add({
            title: t('card.bindFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function doUnbind() {
    if (!unbinding) return
    unbind.mutate(
      { app: app.name, id: unbinding.id },
      {
        onSuccess: (res) => {
          setUnbinding(null)
          setNeedsRestart(!res.app_restarted)
          toast.add({
            title: t(
              res.app_restarted ? 'card.unboundRestarted' : 'card.unbound',
            ),
            type: 'success',
          })
        },
        onError: (error) =>
          toast.add({
            title: t('card.unbindFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function doRestart() {
    restart.mutate(app.name, {
      onSuccess: () => {
        setNeedsRestart(false)
        toast.add({ title: t('card.restarted'), type: 'success' })
      },
      onError: (error) =>
        toast.add({
          title: t('card.restartFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('card.title')}</CardTitle>
        <CardDescription>{t('card.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {buckets.isError ? (
          <Alert variant="destructive">
            <AlertDescription className="flex items-center justify-between gap-3">
              {t('card.loadFailed')}
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void buckets.refetch()}
              >
                {t('card.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {needsRestart ? (
          <Alert>
            <AlertTitle>{t('card.restartTitle')}</AlertTitle>
            <AlertDescription className="flex items-center justify-between gap-3">
              {t('card.restartBody')}
              <Button
                type="button"
                size="sm"
                disabled={restart.isPending}
                onClick={doRestart}
              >
                {restart.isPending ? t('card.restarting') : t('card.restart')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {bound.length === 0 && !buckets.isPending ? (
          <EmptyState
            compact
            icon={<FolderGlassIcon className="size-5.5" aria-hidden="true" />}
            title={t('card.empty')}
            description={t('card.description')}
            action={
              <Button type="button" size="sm" onClick={() => setCreating(true)}>
                <PlusGlassIcon size="1.2em" aria-hidden="true" />
                {t('card.emptyAction')}
              </Button>
            }
          />
        ) : (
          <ul className="space-y-3">
            {bound.map((b) => (
              <BucketRow
                key={b.id}
                bucket={b}
                onBrowse={() => setBrowsing(b)}
                onUnbind={() => setUnbinding(b)}
              />
            ))}
          </ul>
        )}
        {bound.length > 0 || bindable.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setCreating(true)}
            >
              <PlusGlassIcon size="1.2em" aria-hidden="true" />
              {t('card.create')}
            </Button>
            {bindable.length > 0 ? (
              <>
                <Select
                  value={pick}
                  onValueChange={(v) => {
                    if (v) setPick(v)
                  }}
                >
                  <SelectTrigger
                    className="w-56"
                    aria-label={t('card.bindExisting')}
                  >
                    <SelectValue placeholder={t('card.bindPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {bindable.map((b) => (
                      <SelectItem key={b.id} value={b.id}>
                        {b.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  size="sm"
                  disabled={!pick || bind.isPending}
                  onClick={doBind}
                >
                  {t('card.bind')}
                </Button>
              </>
            ) : null}
          </div>
        ) : null}
      </CardContent>
      <CreateBucketDialog
        key={creating ? 'open' : 'closed'}
        appName={app.name}
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => setNeedsRestart(true)}
      />
      <BucketBrowserDialog
        bucket={browsing}
        onClose={() => setBrowsing(null)}
      />
      <ConfirmDialog
        open={unbinding !== null}
        onOpenChange={(open) => {
          if (!open) setUnbinding(null)
        }}
        title={t('card.unbindTitle')}
        description={t('card.unbindDescription', {
          app: app.name,
          prefix: unbinding?.apps[0]?.env_prefix ?? 'S3',
        })}
        confirmLabel={t('card.unbind')}
        tone="destructive"
        pending={unbind.isPending}
        onConfirm={doUnbind}
      />
    </Card>
  )
}

function BucketRow({
  bucket,
  onBrowse,
  onUnbind,
}: Readonly<{
  bucket: StorageBucket
  onBrowse: () => void
  onUnbind: () => void
}>) {
  const { t } = useTranslation('storageBuckets')
  return (
    <li className="space-y-2 rounded-lg border border-border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-foreground">
            {bucket.name}
          </span>
          <Badge variant={bucket.public ? 'warning' : 'outline'}>
            {bucket.public ? t('card.public') : t('card.private')}
          </Badge>
        </div>
        <div className="flex gap-2">
          <Button type="button" size="sm" onClick={onBrowse}>
            <FolderGlassIcon size="1.2em" aria-hidden="true" />
            {t('card.browse')}
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={onUnbind}>
            <UnlinkGlassIcon size="1.2em" aria-hidden="true" />
            {t('card.unbind')}
          </Button>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        {bucket.quota_bytes > 0
          ? t('card.quota', {
              mb: Math.round(bucket.quota_bytes / BYTES_PER_MB),
            })
          : t('card.noQuota')}
        {bucket.lifecycle_days > 0
          ? ` · ${t('card.expire', { days: bucket.lifecycle_days })}`
          : ''}
      </p>
      {bucket.env_keys && bucket.env_keys.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('card.envLabel')}:{' '}
          <code className="font-mono break-all">
            {bucket.env_keys.join(', ')}
          </code>
        </p>
      ) : null}
    </li>
  )
}
