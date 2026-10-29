import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import {
  useBuildCacheSettings,
  useBuildCacheStats,
  useClearBuildCache,
  useRemoveBuildCache,
  useSetBuildCache,
} from '../queries/buildCache'
import { useStorageDestinationsOptional } from '../queries/storage'
import type {
  BuildCacheMode,
  BuildCacheSetting,
  StorageDestination,
} from '../types/storage'

const MODE_LABEL_KEYS = {
  max: 'buildCache.modeMax',
  min: 'buildCache.modeMin',
} as const satisfies Record<BuildCacheMode, string>

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(1)} ${units[unit]}`
}

function CacheUsage({ appName }: { appName: string }) {
  const { t } = useTranslation('settings')
  const stats = useBuildCacheStats(appName, true)
  const clear = useClearBuildCache()
  const [confirming, setConfirming] = useState(false)

  return (
    <div className="space-y-4">
      {stats.isPending ? (
        <div role="status">
          <span className="sr-only">{t('buildCache.checking')}</span>
          <Skeleton decorative className="h-5 w-48" />
        </div>
      ) : stats.isError ? (
        <p className="text-sm text-muted-foreground">
          {t('buildCache.readFailed', { message: stats.error.message })}
        </p>
      ) : (
        <p className="text-sm text-foreground">
          {t('buildCache.objects', {
            count: stats.data.objects,
            plus: stats.data.truncated ? '+' : '',
            size: formatBytes(stats.data.bytes),
          })}
          {stats.data.last_modified
            ? t('buildCache.lastExport', {
                when: new Date(stats.data.last_modified).toLocaleString(),
              })
            : ''}
        </p>
      )}
      {clear.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {clear.error.message}
        </p>
      ) : null}
      <div className="flex gap-2">
        {confirming ? (
          <>
            <Button
              type="button"
              variant="destructive"
              disabled={clear.isPending}
              onClick={() => {
                clear.mutate(appName, {
                  onSuccess: (res) => {
                    setConfirming(false)
                    toast.add({
                      title: res.more
                        ? t('buildCache.clearedMore', { deleted: res.deleted })
                        : t('buildCache.cleared', { deleted: res.deleted }),
                      type: 'success',
                    })
                  },
                })
              }}
            >
              {t('buildCache.deleteLayers')}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setConfirming(false)
              }}
            >
              {t('buildCache.cancel')}
            </Button>
          </>
        ) : (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setConfirming(true)
            }}
          >
            {t('buildCache.clear')}
          </Button>
        )}
      </div>
    </div>
  )
}

function CacheForm({
  appName,
  destinations,
  own,
  inherited,
}: {
  appName: string
  destinations: StorageDestination[]
  own: BuildCacheSetting | undefined
  inherited: BuildCacheSetting | undefined
}) {
  const { t } = useTranslation('settings')
  const seed = own ?? inherited
  const [target, setTarget] = useState(
    seed?.target_id ?? destinations[0]?.id ?? '',
  )
  const [mode, setMode] = useState<BuildCacheMode>(seed?.mode ?? 'max')
  const [enabled, setEnabled] = useState(own?.enabled ?? true)
  const save = useSetBuildCache()
  const remove = useRemoveBuildCache()
  const idPrefix = `build-cache-${appName === '' ? 'global' : appName}`

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (target === '') return
        save.mutate(
          { app_name: appName, target_id: target, mode, enabled },
          {
            onSuccess: () => {
              toast.add({
                title: t('buildCache.saved'),
                type: 'success',
              })
            },
          },
        )
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-target`}>
            {t('buildCache.destination')}
          </FieldLabel>
          <Select
            value={target}
            onValueChange={(v) => {
              if (v !== null) setTarget(v)
            }}
          >
            <SelectTrigger id={`${idPrefix}-target`} className="w-full">
              <SelectValue placeholder={t('buildCache.chooseDestination')} />
            </SelectTrigger>
            <SelectContent>
              {destinations.map((d) => (
                <SelectItem key={d.id} value={d.id}>
                  {d.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-mode`}>
            {t('buildCache.exportMode')}
          </FieldLabel>
          <Select
            value={mode}
            onValueChange={(v) => {
              if (v === 'min' || v === 'max') setMode(v)
            }}
          >
            <SelectTrigger id={`${idPrefix}-mode`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['max', 'min'] as const).map((m) => (
                <SelectItem key={m} value={m}>
                  {t(MODE_LABEL_KEYS[m])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>
      <div className="flex items-center gap-2 text-sm">
        <Switch
          id={`${idPrefix}-enabled`}
          checked={enabled}
          onCheckedChange={setEnabled}
        />
        <label htmlFor={`${idPrefix}-enabled`}>
          {t('buildCache.useRemote')}
        </label>
      </div>
      {appName !== '' ? (
        <FieldDescription>
          {t('buildCache.storedUnder', {
            prefix: own?.key_prefix ?? `build-cache/${appName}/`,
          })}
        </FieldDescription>
      ) : (
        <FieldDescription>{t('buildCache.globalHint')}</FieldDescription>
      )}
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" disabled={save.isPending || target === ''}>
          {own ? t('buildCache.update') : t('buildCache.save')}
        </Button>
        {own ? (
          <Button
            type="button"
            variant="outline"
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate(appName, {
                onSuccess: () => {
                  toast.add({
                    title: t('buildCache.removed'),
                    type: 'success',
                  })
                },
              })
            }}
          >
            {t('buildCache.removeSetting')}
          </Button>
        ) : null}
      </div>
    </form>
  )
}

// BuildKit remote cache controls for one app (appName) or the default for
// every app (appName ''). Bucket credentials never reach the browser.
export function BuildCacheCard({ appName }: { appName: string }) {
  const { t } = useTranslation('settings')
  const destinations = useStorageDestinationsOptional()
  const settings = useBuildCacheSettings()

  const error = settings.error ?? destinations.error
  const notConfigured = error instanceof ApiError && error.status === 501
  const own = settings.data?.find((s) => s.app_name === appName)
  const inherited =
    appName === '' ? undefined : settings.data?.find((s) => s.app_name === '')

  let body
  if (settings.isError || destinations.isError) {
    body = (
      <Alert>
        <AlertTitle>
          {notConfigured
            ? t('buildCache.notConfiguredTitle')
            : t('buildCache.loadFailedTitle')}
        </AlertTitle>
        <AlertDescription>
          {notConfigured
            ? t('buildCache.notConfiguredBody')
            : (error?.message ?? t('buildCache.unknownError'))}
        </AlertDescription>
      </Alert>
    )
  } else if (settings.isPending || destinations.isPending) {
    return <CardSkeleton titleWidth="w-40" rows={2} />
  } else if (destinations.data.length === 0) {
    body = (
      <Alert>
        <AlertTitle>{t('buildCache.noDestinationTitle')}</AlertTitle>
        <AlertDescription>{t('buildCache.noDestinationBody')}</AlertDescription>
      </Alert>
    )
  } else {
    body = (
      <div className="space-y-4">
        {appName !== '' && !own && inherited?.enabled ? (
          <p className="text-sm text-muted-foreground">
            {t('buildCache.usingDefault')}
          </p>
        ) : null}
        {own?.last_warning ? (
          <Alert>
            <AlertTitle>{t('buildCache.ranWithout')}</AlertTitle>
            <AlertDescription>{own.last_warning}</AlertDescription>
          </Alert>
        ) : own?.last_build_at ? (
          <p className="text-sm text-muted-foreground">
            {t('buildCache.lastBuild', {
              when: new Date(own.last_build_at).toLocaleString(),
            })}
          </p>
        ) : null}
        <CacheForm
          key={
            own
              ? `own:${own.updated_at}`
              : `new:${inherited?.target_id}:${inherited?.mode}:${inherited?.updated_at}`
          }
          appName={appName}
          destinations={destinations.data}
          own={own}
          inherited={inherited}
        />
        {appName !== '' && own?.enabled ? (
          <CacheUsage appName={appName} />
        ) : null}
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LayersGlassIcon className="size-4.5" aria-hidden="true" />
          {appName === '' ? t('buildCache.titleAll') : t('buildCache.title')}
        </CardTitle>
        <CardDescription>{t('buildCache.description')}</CardDescription>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}
