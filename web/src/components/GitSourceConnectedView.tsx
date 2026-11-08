import { Suspense, lazy } from 'react'
import { useTranslation } from 'react-i18next'
import { LoaderGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { AppDetail } from '../types/appDetail'
import type { GitSourceResource } from '../types/gitSource'
import { TRIGGER_MODES } from '../lib/gitSourceForm'
import { isSshRepoUrl } from '../lib/gitSshUrl'
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

// Lazy: only SSH repo URLs need the deploy key section.
const GitSshKeySection = lazy(() =>
  import('./GitSshKeySection').then((m) => ({ default: m.GitSshKeySection })),
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
  const { t } = useTranslation('gitSource')
  const additional = Object.keys(source.additional_services ?? {})
  const services = Object.keys(source.services ?? {})
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
        <dt className="text-muted-foreground">{t('connected.repository')}</dt>
        <dd className="min-w-0 truncate font-mono">{source.repo_url}</dd>
        <dt className="text-muted-foreground">{t('connected.branch')}</dt>
        <dd className="font-mono">{source.branch}</dd>
        <dt className="text-muted-foreground">
          {t('connected.deployTrigger')}
        </dt>
        <dd>
          {TRIGGER_MODES.includes(source.trigger_mode)
            ? t(`triggers.${source.trigger_mode}.label`)
            : source.trigger_mode}
        </dd>
        <dt className="text-muted-foreground">{t('connected.deployToken')}</dt>
        <dd>
          {source.has_token
            ? t('connected.tokenConfigured')
            : t('connected.tokenNotSet')}
        </dd>
        {additional.length > 0 ? (
          <>
            <dt className="text-muted-foreground">
              {t('connected.alsoDeploys')}
            </dt>
            <dd className="font-mono">{additional.join(', ')}</dd>
          </>
        ) : null}
        {services.length > 0 ? (
          <>
            <dt className="text-muted-foreground">
              {t('connected.servicesFanout')}
            </dt>
            <dd className="font-mono">{services.join(', ')}</dd>
          </>
        ) : null}
      </dl>

      <GitBuildSettings appName={app.name} source={source} />

      <Suspense fallback={null}>
        <GitSourceReliability appName={app.name} source={source} />
      </Suspense>

      {isSshRepoUrl(source.repo_url) ? (
        <Suspense fallback={null}>
          <GitSshKeySection appName={app.name} />
        </Suspense>
      ) : null}

      <GitSourceCopyField
        id="git-source-webhook-url-connected"
        label={t('connected.webhookUrl')}
        value={`${window.location.origin}${source.webhook_url}`}
        description={t('connected.webhookNote')}
      />

      <div className="flex gap-2">
        <Button type="button" size="sm" variant="outline" onClick={onEdit}>
          {t('connected.edit')}
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
          {t('connected.disconnect')}
        </Button>
      </div>
      <GitAccessValidate
        path={`/api/v1/apps/${encodeURIComponent(app.name)}/git-source/validate`}
      />
    </div>
  )
}
