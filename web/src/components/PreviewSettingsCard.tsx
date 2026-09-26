import { useState } from 'react'
import { CameraIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { InfoTip, SkeletonLine } from './kit'
import { formatBytes } from '../lib/format'
import type { PreviewStatus } from '../types/preview'
import {
  useCapturePreview,
  usePreviewStatus,
  usePrunePreview,
  useRefreshWhenCaptureEnds,
  useSetPreview,
} from '../queries/preview'

function storageText(bytes: number): string {
  return bytes > 0 ? formatBytes(bytes) : '0 B'
}

// PreviewSettingsCard is the per-app opt-in for deploy preview screenshots
// (GET/PUT /api/v1/apps/{name}/preview). Off by default: a preview can show
// internal pages, and each capture runs a short-lived browser container.
export function PreviewSettingsCard({ appName }: { appName: string }) {
  const status = usePreviewStatus(appName)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CameraIcon className="size-4 text-muted-foreground" />
          Deploy preview screenshots
          <InfoTip label="About deploy previews">
            Runs a 140 MB browser container for about 10 seconds after each
            deploy. It loads the app over its private network and saves a small
            thumbnail. The browser image is pulled the first time it is needed
            and removed again once it sits unused.
          </InfoTip>
        </CardTitle>
        <CardDescription>
          A thumbnail of each deploy, shown in deploy history. Off by default.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {status.isPending ? (
          <div className="space-y-2" aria-label="Loading preview settings">
            <SkeletonLine width="60%" />
            <SkeletonLine width="40%" />
          </div>
        ) : status.isError ? (
          <p className="text-sm text-muted-foreground">
            Deploy previews are not available on this control plane.
          </p>
        ) : (
          <PreviewSettingsForm
            key={`${status.data.path}:${status.data.wait_ms}`}
            appName={appName}
            status={status.data}
          />
        )}
      </CardContent>
    </Card>
  )
}

function PreviewSettingsForm({
  appName,
  status,
}: {
  appName: string
  status: PreviewStatus
}) {
  const setPreview = useSetPreview(appName)
  const capture = useCapturePreview(appName)
  const prune = usePrunePreview(appName)
  const [path, setPath] = useState(status.path)
  const [waitMs, setWaitMs] = useState(String(status.wait_ms))
  useRefreshWhenCaptureEnds(appName, status.capturing)

  const waitValue = Number(waitMs)
  const waitValid = Number.isInteger(waitValue) && waitValue >= 0
  const dirty = path !== status.path || waitValue !== status.wait_ms
  const capturing = status.capturing || capture.isPending

  function onError(title: string) {
    return (error: Error) =>
      toast.add({ title, description: error.message, type: 'error' })
  }

  function toggle(next: boolean) {
    setPreview.mutate(
      { enabled: next },
      {
        onSuccess: () =>
          toast.add({
            title: next
              ? 'Deploy previews enabled.'
              : 'Deploy previews disabled.',
            type: 'success',
          }),
        onError: onError('Could not update deploy previews.'),
      },
    )
  }

  function save() {
    setPreview.mutate(
      { path, wait_ms: waitValue },
      {
        onSuccess: () =>
          toast.add({ title: 'Preview settings saved.', type: 'success' }),
        onError: onError('Could not save preview settings.'),
      },
    )
  }

  function pruneNow() {
    prune.mutate(undefined, {
      onSuccess: (res) =>
        toast.add({
          title: 'Previews pruned.',
          description: `Removed ${res.removed}, freed ${storageText(res.freed_bytes)}.`,
          type: 'success',
        }),
      onError: onError('Could not prune previews.'),
    })
  }

  return (
    <div className="space-y-5">
      {!status.server_enabled ? (
        <p className="rounded-md border border-border bg-muted/50 p-3 text-sm text-muted-foreground">
          Previews are switched off on this server. Set{' '}
          <code className="font-mono text-xs">APP_PREVIEW_ENABLED=true</code>{' '}
          and restart the control plane to allow them.
        </p>
      ) : null}

      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-foreground">Enabled</p>
          <p className="text-sm text-muted-foreground">
            Screenshots can show internal pages. The page is loaded without
            cookies, so login pages are skipped.
          </p>
        </div>
        <Switch
          checked={status.enabled}
          onCheckedChange={toggle}
          disabled={setPreview.isPending}
          aria-label="Deploy preview screenshots enabled"
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="preview-path" className="flex items-center gap-1.5">
            Page to capture
            <InfoTip label="About the capture path">
              An absolute path on this app, such as / or /pricing. The browser
              reaches the app by its service name, never by a URL you supply.
            </InfoTip>
          </Label>
          <Input
            id="preview-path"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/"
            spellCheck={false}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="preview-wait" className="flex items-center gap-1.5">
            Extra wait (ms)
            <InfoTip label="About the extra wait">
              Milliseconds to let the page settle after it loads, for apps that
              render late. Up to 10000.
            </InfoTip>
          </Label>
          <Input
            id="preview-wait"
            inputMode="numeric"
            value={waitMs}
            onChange={(e) => setWaitMs(e.target.value)}
            aria-invalid={!waitValid}
          />
        </div>
      </div>
      <div>
        <Button
          size="sm"
          onClick={save}
          disabled={!dirty || !waitValid || setPreview.isPending}
        >
          Save
        </Button>
      </div>

      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">Storage used by this app</dt>
          <dd className="font-medium text-foreground">
            {storageText(status.storage.app_bytes)} in{' '}
            {status.storage.app_count} previews
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">All apps</dt>
          <dd className="font-medium text-foreground">
            {storageText(status.storage.total_bytes)} of {status.max_total_mb}{' '}
            MB
          </dd>
        </div>
        <div>
          <dt className="flex items-center gap-1.5 text-muted-foreground">
            Retention
            <InfoTip label="About retention">
              The newest {status.keep_per_app} previews of each app are kept,
              plus the one for the live release. Anything older than{' '}
              {status.ttl_days} days is deleted, and the oldest previews go
              first if all apps together pass {status.max_total_mb} MB.
            </InfoTip>
          </dt>
          <dd className="font-medium text-foreground">
            Last {status.keep_per_app} plus live, {status.ttl_days} days
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Browser image</dt>
          <dd
            className="truncate font-mono text-xs text-foreground"
            title={status.image}
          >
            {status.image}
          </dd>
        </div>
      </dl>

      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() =>
            capture.mutate(undefined, {
              onError: onError('Could not start a preview capture.'),
            })
          }
          disabled={!status.enabled || !status.server_enabled || capturing}
        >
          <CameraIcon className="size-3.5" data-icon="inline-start" />
          {capturing ? 'Capturing...' : 'Recapture now'}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={pruneNow}
          disabled={prune.isPending}
        >
          <TrashIcon className="size-3.5" data-icon="inline-start" />
          {prune.isPending ? 'Pruning...' : 'Prune now'}
        </Button>
      </div>
    </div>
  )
}
