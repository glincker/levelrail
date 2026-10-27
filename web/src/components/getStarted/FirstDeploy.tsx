import { useState } from 'react'
import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  GitBranchIcon,
  RocketLaunchIcon,
  SquaresFourIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { EnableHttpsCard } from '../EnableHttpsCard'
import { ApiError } from '../../lib/apiError'
import { SAMPLE_APP } from '../../lib/firstApp'
import { healthCheckFrom } from '../../lib/healthCheckDefaults'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useCreateApp } from '../../queries/apps'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { FirstAppStatus } from './FirstAppStatus'
import { useFirstAppPolling } from './useFirstAppPolling'

const HTTP_CONFLICT = 409

function PathCard({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode
  title: string
  description: string
  action: ReactNode
}) {
  return (
    <Card>
      <CardContent className="flex h-full items-center gap-3 sm:flex-col sm:items-stretch sm:gap-2">
        <span aria-hidden="true" className="shrink-0">
          {icon}
        </span>
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className="text-sm font-medium text-foreground">{title}</p>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
        <div className="shrink-0 sm:shrink [&_button]:min-h-11 sm:[&_button]:min-h-0">
          {action}
        </div>
      </CardContent>
    </Card>
  )
}

/** FirstDeploy is the one first-run path: pick how to deploy, see the address it will get, and end on a live app with a link. */
export function FirstDeploy({
  onStarted,
  onDone,
}: {
  onStarted: () => void
  onDone: () => void
}) {
  const { t } = useTranslation('getStarted')
  const isRoot = useIsRoot()
  const createApp = useCreateApp()
  const [mountedAt] = useState(() => Date.now())
  const [tracking, setTracking] = useState<number | null>(null)
  const { app, phase } = useFirstAppPolling(
    tracking ?? mountedAt,
    tracking !== null,
  )
  const { data: https } = useQuery({
    ...httpsStatusQueryOptions(),
    enabled: isRoot,
    retry: false,
  })

  function track() {
    setTracking(Date.now())
    onStarted()
  }

  function deploySample() {
    createApp.mutate(
      {
        name: SAMPLE_APP.name,
        image: SAMPLE_APP.image,
        port: SAMPLE_APP.port,
        health: healthCheckFrom(true, SAMPLE_APP.readinessPath),
      },
      {
        onSuccess: track,
        onError: (err) => {
          if (err instanceof ApiError && err.status === HTTP_CONFLICT) track()
        },
      },
    )
  }

  const conflict =
    createApp.error instanceof ApiError &&
    createApp.error.status === HTTP_CONFLICT
  const createError =
    createApp.error && !conflict ? createApp.error.message : ''
  const live = Boolean(app) && phase === 'live'
  const preview = https?.suggested_domain
    ? `${https.state === 'issued' ? 'https' : 'http'}://${SAMPLE_APP.name}.${https.suggested_domain}`
    : ''

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="flex items-center gap-2 text-xl font-semibold text-foreground">
          <RocketLaunchIcon
            aria-hidden="true"
            className="size-6 text-primary"
          />
          {live ? t('first.titleLive') : t('first.title')}
        </h1>
        {live ? null : (
          <p className="max-w-prose text-sm text-muted-foreground">
            {t('first.subtitle')}
          </p>
        )}
      </header>

      {app ? (
        <FirstAppStatus app={app} phase={phase} onDone={onDone} />
      ) : (
        <>
          <div className="grid gap-2 sm:grid-cols-3 sm:gap-3">
            <PathCard
              icon={<RocketLaunchIcon className="size-5 text-primary" />}
              title={t('first.sample.title')}
              description={t('first.sample.description', {
                image: SAMPLE_APP.image,
                path: SAMPLE_APP.readinessPath,
              })}
              action={
                <Button
                  size="sm"
                  onClick={deploySample}
                  disabled={createApp.isPending}
                >
                  {createApp.isPending
                    ? t('first.sample.creating')
                    : createError
                      ? t('first.sample.retry')
                      : t('first.sample.cta')}
                </Button>
              }
            />
            <PathCard
              icon={
                <SquaresFourIcon className="size-5 text-muted-foreground" />
              }
              title={t('first.template.title')}
              description={t('first.template.description')}
              action={
                <CreateResourceWizard
                  initialSelected="browse-templates"
                  trigger={
                    <Button size="sm" variant="outline">
                      {t('first.template.cta')}
                    </Button>
                  }
                />
              }
            />
            <PathCard
              icon={<GitBranchIcon className="size-5 text-muted-foreground" />}
              title={t('first.repo.title')}
              description={t('first.repo.description')}
              action={
                <CreateResourceWizard
                  initialSelected="dockerfile-git"
                  trigger={
                    <Button size="sm" variant="outline">
                      {t('first.repo.cta')}
                    </Button>
                  }
                />
              }
            />
          </div>
          <p className="flex flex-wrap items-center gap-x-4 text-xs text-muted-foreground sm:gap-y-1">
            <Link
              to="/settings/import-platform"
              className="inline-flex min-h-11 items-center gap-1 text-foreground underline-offset-2 sm:min-h-0 hover:underline focus-visible:outline-2 focus-visible:outline-ring"
            >
              {t('first.more.import')}
              <ArrowRightIcon aria-hidden="true" className="size-3" />
            </Link>
            <Link
              to="/settings/github-app"
              className="inline-flex min-h-11 items-center gap-1 text-foreground underline-offset-2 sm:min-h-0 hover:underline focus-visible:outline-2 focus-visible:outline-ring"
            >
              {t('first.more.git')}
              <ArrowRightIcon aria-hidden="true" className="size-3" />
            </Link>
          </p>
          {createError ? (
            <p role="alert" className="text-xs text-destructive">
              {createError}
            </p>
          ) : null}
          {preview ? (
            <p className="text-xs text-muted-foreground">
              {t('first.url.preview', { url: preview })}
            </p>
          ) : null}
        </>
      )}

      {isRoot ? (
        <div id="enable-https">
          <EnableHttpsCard compact />
        </div>
      ) : null}
    </div>
  )
}
