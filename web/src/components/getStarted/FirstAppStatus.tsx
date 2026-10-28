import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  CircleNotchIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import type { AppListEntry } from '../../types/appDetail'
import { useAppNetwork } from '../../queries/appNetwork'
import { useAppUrls } from '../../queries/appUrls'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useDiagnosis } from '../../queries/diagnosis'
import { SAMPLE_APP, firstAppAddress } from '../../lib/firstApp'
import { deployAttemptsQueryOptions } from '../../queries/deployAttempts'
import { useTriggerDeploy } from '../../queries/deploys'
import type { AppPhase } from '../../lib/firstApp'

/** FirstAppStatus renders the tracked app's deploy progress, its live URL, or its diagnosis. */
export function FirstAppStatus({
  app,
  phase,
  onDone,
}: {
  app: AppListEntry
  phase: AppPhase
  onDone?: () => void
}) {
  const { t } = useTranslation('getStarted')
  const { data: network } = useAppNetwork(app.name)
  const { data: urls } = useAppUrls(app.name)
  const isRoot = useIsRoot()
  const { data: https } = useQuery({
    ...httpsStatusQueryOptions(),
    enabled: isRoot,
    retry: false,
  })
  const address = firstAppAddress(
    app,
    network,
    urls,
    window.location.hostname,
    https?.state === 'issued',
  )
  const url = address?.url
  // On the dashboard the card itself is the destination, so the shortcut would be a no-op link.
  const onDashboard = window.location.pathname === '/'
  const failed = phase === 'failed'
  const { data: attempts } = useQuery({
    ...deployAttemptsQueryOptions(app.name),
    enabled: failed,
  })
  const failure = attempts?.find((a) => a.failure)?.failure
  const redeploy = useTriggerDeploy(app.name)
  const [retried, setRetried] = useState(false)
  const canRetry = app.name === SAMPLE_APP.name && failure?.retryable !== false
  const { data: diagnosis, isLoading: diagnosing } = useDiagnosis(
    app.name,
    undefined,
    failed,
  )

  // The live address pill already opens the app, so the extra Open button only shows without one.
  const showOpenApp = phase !== 'live' || !url
  const appLinks = (
    <div className="flex flex-wrap gap-2">
      {showOpenApp ? (
        <Button
          size="sm"
          className="max-sm:min-h-11"
          variant="outline"
          render={<Link to="/apps/$name" params={{ name: app.name }} />}
        >
          {t('first.status.openApp')}
        </Button>
      ) : null}
      <Button
        size="sm"
        className="max-sm:min-h-11"
        variant="outline"
        render={<Link to="/apps/$name/logs" params={{ name: app.name }} />}
      >
        {t('first.status.viewLogs')}
      </Button>
      {phase === 'live' ? (
        <Button
          size="sm"
          className="max-sm:min-h-11"
          variant="outline"
          render={<Link to="/apps/$name/go-live" params={{ name: app.name }} />}
        >
          {t('first.status.goLive')}
        </Button>
      ) : null}
    </div>
  )

  if (phase === 'live') {
    return (
      <div className="space-y-3 rounded-lg border border-tone-success-border bg-tone-success-soft p-4">
        <p className="flex items-center gap-2 text-sm text-foreground">
          <CheckCircleIcon
            aria-hidden="true"
            className="size-5 shrink-0 text-tone-success"
          />
          {t('first.status.liveDetail', { name: app.name })}
        </p>
        {url ? (
          <Button
            size="sm"
            className="max-sm:min-h-11"
            render={<a href={url} target="_blank" rel="noreferrer" />}
            nativeButton={false}
          >
            {url}
            <ArrowSquareOutIcon />
          </Button>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('first.status.noAddress')}
          </p>
        )}
        {address?.certWarning ? (
          <p className="text-xs text-muted-foreground">
            {t('first.status.certNote')}
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {appLinks}
          {onDone && !onDashboard ? (
            <Button
              size="sm"
              className="max-sm:min-h-11"
              variant="ghost"
              onClick={onDone}
            >
              {t('first.status.done')}
            </Button>
          ) : null}
        </div>
      </div>
    )
  }

  if (failed) {
    return (
      <div className="space-y-3 rounded-lg border border-destructive/40 p-4">
        <p className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <WarningCircleIcon className="size-5 text-destructive" />
          {t('first.status.failed', { name: app.name })}
        </p>
        {failure ? (
          <div className="space-y-1 text-sm">
            <p className="text-foreground">{failure.cause}</p>
            {failure.suggested_fix ? (
              <p className="text-muted-foreground">{failure.suggested_fix}</p>
            ) : null}
          </div>
        ) : null}
        {diagnosing && !failure ? (
          <p className="text-xs text-muted-foreground">
            {t('first.status.diagnosing')}
          </p>
        ) : null}
        {diagnosis && !failure ? (
          <div className="space-y-1.5 text-sm">
            <p className="text-foreground">{diagnosis.explanation}</p>
            {diagnosis.suggestion ? (
              <p className="text-muted-foreground">{diagnosis.suggestion}</p>
            ) : null}
            {diagnosis.matched_signals.slice(0, 3).map((s) => (
              <pre
                key={`${s.source}:${s.excerpt}`}
                className="overflow-x-auto rounded-md bg-muted/50 p-2 text-xs whitespace-pre-wrap"
              >
                {s.excerpt}
              </pre>
            ))}
          </div>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {canRetry ? (
            <Button
              size="sm"
              className="max-sm:min-h-11"
              disabled={redeploy.isPending || retried}
              onClick={() =>
                redeploy.mutate(
                  { image: SAMPLE_APP.image, rerunOf: failure?.deploy_id },
                  { onSuccess: () => setRetried(true) },
                )
              }
            >
              {t('first.status.retry')}
            </Button>
          ) : null}
          {appLinks}
          {onDone && !onDashboard ? (
            <Button
              size="sm"
              className="max-sm:min-h-11"
              variant="ghost"
              onClick={onDone}
            >
              {t('first.status.done')}
            </Button>
          ) : null}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <p className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <CircleNotchIcon className="size-5 animate-spin text-primary motion-reduce:animate-none" />
        {t('first.status.deploying', { name: app.name })}
      </p>
      <p className="text-sm text-muted-foreground">
        {t('first.status.deployingDetail', { status: app.status.label })}
      </p>
      {phase === 'deploying' ? (
        <p className="text-xs text-muted-foreground">
          {t('first.status.expectedWait')}
        </p>
      ) : null}
      {phase === 'slow' ? (
        <p className="text-xs text-muted-foreground">
          {t('first.status.slow')}
        </p>
      ) : null}
      {appLinks}
    </div>
  )
}
