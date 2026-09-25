import { useState } from 'react'
import { StackIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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

const MODE_LABELS: Record<BuildCacheMode, string> = {
  max: 'max (cache every layer, fastest rebuilds)',
  min: 'min (cache final layers only, smaller bucket)',
}

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
  const stats = useBuildCacheStats(appName, true)
  const clear = useClearBuildCache()
  const [confirming, setConfirming] = useState(false)

  return (
    <div className="space-y-3 rounded-lg border p-4">
      {stats.isPending ? (
        <p className="text-sm text-muted-foreground">Checking the bucket...</p>
      ) : stats.isError ? (
        <p className="text-sm text-muted-foreground">
          Could not read the bucket: {stats.error.message}
        </p>
      ) : (
        <p className="text-sm text-foreground">
          {stats.data.objects}
          {stats.data.truncated ? '+' : ''} objects,{' '}
          {formatBytes(stats.data.bytes)}
          {stats.data.last_modified
            ? `, last export ${new Date(stats.data.last_modified).toLocaleString()}`
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
                        ? `Deleted ${res.deleted} objects. More remain, clear again.`
                        : `Cache cleared (${res.deleted} objects).`,
                      type: 'success',
                    })
                  },
                })
              }}
            >
              Delete cached layers
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setConfirming(false)
              }}
            >
              Cancel
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
            Clear cache
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
              toast.add({ title: 'Build cache saved.', type: 'success' })
            },
          },
        )
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-target`}>Destination</FieldLabel>
          <Select
            value={target}
            onValueChange={(v) => {
              if (v !== null) setTarget(v)
            }}
          >
            <SelectTrigger id={`${idPrefix}-target`} className="w-full">
              <SelectValue placeholder="Choose a destination" />
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
          <FieldLabel htmlFor={`${idPrefix}-mode`}>Export mode</FieldLabel>
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
                  {MODE_LABELS[m]}
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
        <label htmlFor={`${idPrefix}-enabled`}>Use the remote cache</label>
      </div>
      {appName !== '' ? (
        <FieldDescription>
          Layers are stored under {own?.key_prefix ?? `build-cache/${appName}/`}
          . Turn this off to opt this app out of the default.
        </FieldDescription>
      ) : (
        <FieldDescription>
          Applies to every app without its own setting. Each app caches under
          build-cache/&lt;app&gt;/.
        </FieldDescription>
      )}
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" disabled={save.isPending || target === ''}>
          {own ? 'Update' : 'Save'}
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
                    title: 'Build cache setting removed.',
                    type: 'success',
                  })
                },
              })
            }}
          >
            Remove setting
          </Button>
        ) : null}
      </div>
    </form>
  )
}

// BuildKit remote cache controls for one app (appName) or the default for
// every app (appName ''). Bucket credentials never reach the browser.
export function BuildCacheCard({ appName }: { appName: string }) {
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
            ? 'Build cache is not configured on this server'
            : 'Could not load build cache settings'}
        </AlertTitle>
        <AlertDescription>
          {notConfigured
            ? 'Set APP_MASTER_KEY and restart the control plane to connect object storage.'
            : (error?.message ?? 'Unknown error')}
        </AlertDescription>
      </Alert>
    )
  } else if (settings.isPending || destinations.isPending) {
    body = <p className="text-sm text-muted-foreground">Loading...</p>
  } else if (destinations.data.length === 0) {
    body = (
      <Alert>
        <AlertTitle>No storage destination yet</AlertTitle>
        <AlertDescription>
          Connect a bucket under Settings, Storage destinations, then use it as
          a build cache here.
        </AlertDescription>
      </Alert>
    )
  } else {
    body = (
      <div className="space-y-4">
        {appName !== '' && !own && inherited?.enabled ? (
          <p className="text-sm text-muted-foreground">
            Using the default build cache for all apps. Save a setting below to
            override it for this app.
          </p>
        ) : null}
        {own?.last_warning ? (
          <Alert>
            <AlertTitle>Last build ran without the cache</AlertTitle>
            <AlertDescription>{own.last_warning}</AlertDescription>
          </Alert>
        ) : own?.last_build_at ? (
          <p className="text-sm text-muted-foreground">
            Last build with the cache:{' '}
            {new Date(own.last_build_at).toLocaleString()}
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
          <StackIcon className="size-4" aria-hidden="true" />
          Build cache
          {appName === '' ? ' for all apps' : ''}
        </CardTitle>
        <CardDescription>
          Keep BuildKit layers in a storage destination so rebuilds skip
          unchanged steps. A cache problem never fails a build, it is reported
          as a warning.
        </CardDescription>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}
