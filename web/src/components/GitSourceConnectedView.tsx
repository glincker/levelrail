import { Suspense, lazy } from 'react'
import { LoaderGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { AppDetail } from '../types/appDetail'
import type { GitSourceResource } from '../types/gitSource'
import { TRIGGER_MODES } from '../lib/gitSourceForm'
import { GitAccessValidate } from './GitAccessValidate'
import { GitBuildSettings } from './GitBuildSettings'
import { GitSourceCopyField } from './GitSourceCopyField'
import { RotateWebhookSecretDialog } from './RotateWebhookSecretDialog'

// Lazy: the reliability panel only shows for connected sources, so keep it out of the source route chunk.
const GitSourceReliability = lazy(() =>
  import('./GitSourceReliability').then((m) => ({
    default: m.GitSourceReliability,
  })),
)

export function GitSourceConnectedView({
  app,
  source,
  disconnecting,
  onEdit,
  onDisconnect,
}: {
  app: AppDetail
  source: GitSourceResource
  disconnecting: boolean
  onEdit: () => void
  onDisconnect: () => void
}) {
  const additional = Object.keys(source.additional_services ?? {})
  const services = Object.keys(source.services ?? {})
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
        <dt className="text-muted-foreground">{/* i18n-ignore */}Repository</dt>
        <dd className="min-w-0 truncate font-mono">{source.repo_url}</dd>
        <dt className="text-muted-foreground">{/* i18n-ignore */}Branch</dt>
        <dd className="font-mono">{source.branch}</dd>
        <dt className="text-muted-foreground">
          {/* i18n-ignore */}Deploy trigger
        </dt>
        <dd>
          {TRIGGER_MODES.find((m) => m.value === source.trigger_mode)?.label ??
            source.trigger_mode}
        </dd>
        <dt className="text-muted-foreground">
          {/* i18n-ignore */}Deploy token
        </dt>
        <dd>{source.has_token ? 'Configured' : 'Not set (public repo)'}</dd>
        {additional.length > 0 ? (
          <>
            <dt className="text-muted-foreground">
              {/* i18n-ignore */}Also deploys
            </dt>
            <dd className="font-mono">{additional.join(', ')}</dd>
          </>
        ) : null}
        {services.length > 0 ? (
          <>
            <dt className="text-muted-foreground">
              {/* i18n-ignore */}Services fan-out
            </dt>
            <dd className="font-mono">{services.join(', ')}</dd>
          </>
        ) : null}
      </dl>

      <GitBuildSettings appName={app.name} source={source} />

      <Suspense fallback={null}>
        <GitSourceReliability appName={app.name} source={source} />
      </Suspense>

      <GitSourceCopyField
        id="git-source-webhook-url-connected"
        label="Webhook URL" // i18n-ignore
        value={`${window.location.origin}${source.webhook_url}`}
        description="The webhook secret was only ever shown once, at connect time. Rotate it below to mint a new one, or disconnect and reconnect to also change the repository or branch." // i18n-ignore
      />

      <div className="flex gap-2">
        <Button type="button" size="sm" variant="outline" onClick={onEdit}>
          Edit
        </Button>
        <RotateWebhookSecretDialog appName={app.name} />
        <Button
          type="button"
          size="sm"
          variant="destructive"
          disabled={disconnecting}
          onClick={onDisconnect}
        >
          {disconnecting ? (
            <LoaderGlassIcon className="size-5 animate-spin" />
          ) : null}
          Disconnect
        </Button>
      </div>
      <GitAccessValidate
        path={`/api/v1/apps/${encodeURIComponent(app.name)}/git-source/validate`}
      />
    </div>
  )
}
