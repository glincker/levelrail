import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { GoLiveCta } from '../golive/GoLiveCta'
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
import { openableUrl } from '../../lib/firstApp'
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
  const url = openableUrl(
    app,
    network,
    urls,
    window.location.hostname,
    https?.state === 'issued',
  )
  const insecure =
    Boolean(https) && https?.state !== 'issued' && url?.startsWith('http:')
  const failed = phase === 'failed'
  const { data: diagnosis, isLoading: diagnosing } = useDiagnosis(
    app.name,
    undefined,
    failed,
  )

  const appLinks = (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/apps/$name" params={{ name: app.name }} />}
      >
        {t('first.status.openApp')}
      </Button>
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/apps/$name/logs" params={{ name: app.name }} />}
      >
        {t('first.status.viewLogs')}
      </Button>
    </div>
  )

  if (phase === 'live') {
    return (
      <div className="space-y-3 rounded-lg border border-tone-success-border bg-tone-success-soft p-4">
        <p className="flex items-center gap-2 text-base font-semibold text-foreground">
          <CheckCircleIcon className="size-5 text-tone-success" />
          {t('first.status.live')}
        </p>
        <p className="text-sm text-muted-foreground">
          {t('first.status.liveDetail', { name: app.name })}
        </p>
        {url ? (
          <Button
            size="sm"
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
        {insecure ? (
          <p className="text-xs text-muted-foreground">
            {t('first.status.notSecure')}{' '}
            <a
              href="#enable-https"
              className="text-foreground underline underline-offset-2"
            >
              {t('first.status.enableHttps')}
            </a>
          </p>
        ) : null}
        <GoLiveCta app={app.name} />
        <div className="flex flex-wrap gap-2">
          {appLinks}
          {onDone ? (
            <Button size="sm" variant="ghost" onClick={onDone}>
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
        {diagnosing ? (
          <p className="text-xs text-muted-foreground">
            {t('first.status.diagnosing')}
          </p>
        ) : null}
        {diagnosis ? (
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
          {appLinks}
          {onDone ? (
            <Button size="sm" variant="ghost" onClick={onDone}>
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
