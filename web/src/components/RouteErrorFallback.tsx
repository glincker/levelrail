import { useContext, useState, type ReactNode } from 'react'
import { Link, useParams, useRouter } from '@tanstack/react-router'
import type { ErrorComponentProps } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CopyGlassIcon,
  HouseGlassIcon,
  PlugGlassIcon,
  RotateCwGlassIcon,
  SocialGithubGlassIcon,
} from '@thesvg/ui-react'
import { ApiError, routeErrorMessage } from '@/lib/apiError'
import { isConnectivityError } from '@/lib/connectionState'
import { BrandContext } from '@/lib/brandContext'
import { appListQueryOptions } from '@/queries/apps'
import { databaseListQueryOptions } from '@/queries/databases'
import { ForbiddenState } from './teams/ForbiddenState'
import { NotFoundState, type NotFoundResource } from './NotFoundState'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// Plain text for a bug report: no tokens or headers, only what the user sees.
function errorReport(error: unknown, header: string): string {
  const api = error instanceof ApiError ? error : undefined
  return [
    header,
    `message: ${routeErrorMessage(error)}`,
    api ? `status: ${api.status}` : null,
    api?.code ? `code: ${api.code}` : null,
    api?.requestId ? `request_id: ${api.requestId}` : null,
    `url: ${window.location.pathname}`,
    `time: ${new Date().toISOString()}`,
  ]
    .filter((l): l is string => l !== null)
    .join('\n')
}

function CopyDetailsButton({ error }: { error: unknown }) {
  const { t } = useTranslation('common')
  const [copied, setCopied] = useState(false)
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      onClick={() => {
        void navigator.clipboard
          .writeText(errorReport(error, t('routeError.detailsHeader')))
          .then(() => {
            setCopied(true)
          })
          .catch(() => undefined)
      }}
    >
      {copied ? (
        <CheckGlassIcon size="1.2em" aria-hidden="true" />
      ) : (
        <CopyGlassIcon size="1.2em" aria-hidden="true" />
      )}
      {copied ? t('routeError.copied') : t('routeError.copy')}
    </Button>
  )
}

function ReportLink() {
  const { t } = useTranslation('common')
  const brand = useContext(BrandContext)
  if (!brand?.SupportURL) return null
  return (
    <a
      href={brand.SupportURL}
      target="_blank"
      rel="noreferrer"
      className={buttonVariants({ variant: 'ghost', size: 'sm' })}
    >
      <SocialGithubGlassIcon size="1.2em" aria-hidden="true" />
      {t('routeError.report')}
    </a>
  )
}

function RetryButton({ label, reset }: { label: string; reset: () => void }) {
  const router = useRouter()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={() => {
        // Reset alone re-renders the failed match; invalidate re-runs the loaders.
        void router.invalidate()
        reset()
      }}
    >
      <RotateCwGlassIcon size="1.2em" aria-hidden="true" />
      {label}
    </Button>
  )
}

function ErrorCard({
  icon,
  title,
  children,
}: {
  icon: ReactNode
  title: string
  children: ReactNode
}) {
  return (
    <div className="flex min-h-[40vh] items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-center gap-2">
            {icon}
            <CardTitle>{title}</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">{children}</CardContent>
      </Card>
    </div>
  )
}

function RequestId({ error }: { error: unknown }) {
  const { t } = useTranslation('common')
  if (!(error instanceof ApiError) || !error.requestId) return null
  return (
    <p className="font-mono text-xs text-muted-foreground">
      {t('routeError.requestId', { id: error.requestId })}
    </p>
  )
}

// A TypeError from fetch itself means no response; any other TypeError is a bug.
const FETCH_FAILURE =
  /failed to fetch|networkerror|load failed|network request failed/i

function isUnreachable(error: unknown): boolean {
  if (error instanceof ApiError) {
    return isConnectivityError(error) || error.status >= 500
  }
  return error instanceof TypeError && FETCH_FAILURE.test(error.message)
}

// Reads names the user already has cached; never blocks the error view.
function useCandidates(resource: NotFoundResource): string[] {
  const apps = useQuery({
    ...appListQueryOptions(),
    enabled: resource === 'app',
    retry: false,
  })
  const dbs = useQuery({
    ...databaseListQueryOptions(),
    enabled: resource === 'database',
    retry: false,
  })
  if (resource === 'app') return (apps.data ?? []).map((a) => a.name)
  if (resource === 'database') return (dbs.data ?? []).map((d) => d.name)
  return []
}

const PARAM_FOR: Record<NotFoundResource, string> = {
  app: 'name',
  database: 'name',
  domain: 'domain',
  template: 'id',
  project: 'id',
  generic: '',
}

/** 404 view for a route whose URL param names the missing resource. */
export function ResourceNotFound({ resource }: { resource: NotFoundResource }) {
  const params: Record<string, string | undefined> = useParams({
    strict: false,
  })
  const key = PARAM_FOR[resource]
  const candidates = useCandidates(resource)
  return (
    <NotFoundState
      resource={resource}
      name={key ? params[key] : undefined}
      candidates={candidates}
    />
  )
}

// Router-level defaultErrorComponent (see main.tsx): classifies the error so
// a missing page, an unreachable server and a bug each get the right view.
export function RouteErrorFallback({
  error,
  reset,
  resource = 'generic',
}: ErrorComponentProps & { resource?: NotFoundResource }) {
  const { t } = useTranslation('common')
  if (error instanceof ApiError && error.status === 403) {
    return <ForbiddenState message={error.message} />
  }
  if (error instanceof ApiError && error.status === 404) {
    return <ResourceNotFound resource={resource} />
  }
  if (isUnreachable(error)) {
    return (
      <ErrorCard
        icon={
          <PlugGlassIcon
            className="size-5.5 text-destructive"
            aria-hidden="true"
          />
        }
        title={t('routeError.unreachable.title')}
      >
        <p className="text-sm text-muted-foreground">
          {t('routeError.unreachable.body')}
        </p>
        <RequestId error={error} />
        <div className="flex flex-wrap gap-2">
          <RetryButton
            label={t('routeError.unreachable.retry')}
            reset={reset}
          />
          <CopyDetailsButton error={error} />
        </div>
      </ErrorCard>
    )
  }
  return (
    <ErrorCard
      icon={
        <CircleAlertGlassIcon
          className="size-5.5 text-destructive"
          aria-hidden="true"
        />
      }
      title={t('routeError.generic.title')}
    >
      <p className="text-sm text-muted-foreground">
        {t('routeError.generic.body')}
      </p>
      <details className="text-sm">
        <summary className="cursor-pointer text-muted-foreground">
          {t('routeError.details')}
        </summary>
        <p className="mt-2 break-words font-mono text-xs text-foreground">
          {routeErrorMessage(error)}
        </p>
        <RequestId error={error} />
      </details>
      <div className="flex flex-wrap gap-2">
        <RetryButton label={t('routeError.generic.retry')} reset={reset} />
        <Link to="/">
          <Button type="button" variant="ghost" size="sm">
            <HouseGlassIcon size="1.2em" aria-hidden="true" />
            {t('routeError.generic.home')}
          </Button>
        </Link>
        <CopyDetailsButton error={error} />
        <ReportLink />
      </div>
    </ErrorCard>
  )
}
