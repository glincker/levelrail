import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  LockGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { httpsStatusQueryOptions, useEnableHttps } from '../queries/httpsStatus'
import type { HttpsStatus } from '../queries/httpsStatus'
import { useUpdateDashboardUrl } from '../queries/dashboardUrl'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'

const HTTPS_SCHEME = 'https:'

function useAutoSaveDashboardUrl(status: HttpsStatus | undefined) {
  const update = useUpdateDashboardUrl()
  const done = useRef(false)
  const { t } = useTranslation('https')
  useEffect(() => {
    if (done.current || !status || status.state !== 'issued' || status.staging)
      return
    const target = `https://${status.domain}`
    // Saved only from a tab already on that https URL, so it is proven to
    // work before sign-in over plain HTTP gets refused.
    if (window.location.protocol !== HTTPS_SCHEME) return
    if (window.location.host !== status.domain) return
    if (status.dashboard_url === target) return
    done.current = true
    update.mutate(
      { dashboard_url: target },
      {
        onSuccess: () =>
          toast.add({
            title: t('card.dashboardUrlSaved', { url: target }),
            type: 'success',
          }),
        onError: () =>
          toast.add({ title: t('card.dashboardUrlFailed'), type: 'error' }),
      },
    )
  }, [status, update, t])
}

function EnableForm({
  suggestedDomain,
  initialEmail,
  pending,
  error,
  onSubmit,
}: {
  suggestedDomain: string
  initialEmail: string
  pending: boolean
  error?: string
  onSubmit: (email: string, staging: boolean) => void
}) {
  const { t } = useTranslation('https')
  const [email, setEmail] = useState(initialEmail)
  const [staging, setStaging] = useState(false)
  function submit(e: FormEvent) {
    e.preventDefault()
    onSubmit(email.trim(), staging)
  }
  return (
    <form onSubmit={submit} className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        {t('card.description', { domain: suggestedDomain })}
      </p>
      <p className="max-w-prose text-sm text-muted-foreground">
        {t('card.httpSignInNote')}
      </p>
      <div className="max-w-sm space-y-1.5">
        <Label htmlFor="enable-https-email">{t('card.emailLabel')}</Label>
        <Input
          id="enable-https-email"
          type="email"
          autoComplete="email"
          placeholder="you@yourdomain.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <p className="text-xs text-muted-foreground">{t('card.emailHelp')}</p>
      </div>
      <div className="flex items-start gap-2">
        <Checkbox
          id="enable-https-staging"
          checked={staging}
          onCheckedChange={(v) => setStaging(v === true)}
        />
        <div className="space-y-0.5">
          <Label htmlFor="enable-https-staging">{t('card.stagingLabel')}</Label>
          <p className="text-xs text-muted-foreground">
            {t('card.stagingHelp')}
          </p>
        </div>
      </div>
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
      <Button type="submit" disabled={pending || email.trim() === ''}>
        {pending ? t('card.enabling') : t('card.enable')}
      </Button>
    </form>
  )
}

function StatusBody({
  status,
  preflight,
  onRetry,
  onRecheck,
  rechecking,
}: {
  status: HttpsStatus
  preflight?: HttpsStatus['preflight']
  onRetry: () => void
  onRecheck: () => void
  rechecking: boolean
}) {
  const { t } = useTranslation('https')
  if (status.state === 'pending') {
    const closed = (preflight ?? []).filter((p) => !p.reachable)
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm" role="status">
          <LoaderCircleGlassIcon className="size-4.5 animate-spin" />
          {t('card.pending', { domain: status.domain })}
        </p>
        {closed.length > 0 ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertTitle>{t('card.preflightTitle')}</AlertTitle>
            <AlertDescription>
              {t('card.preflightBody', {
                ports: closed.map((p) => p.port).join(', '),
              })}
            </AlertDescription>
          </Alert>
        ) : null}
      </div>
    )
  }
  if (status.state === 'issued') {
    const date = status.not_after
      ? new Date(status.not_after).toLocaleDateString()
      : ''
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm font-medium">
          <CircleCheckGlassIcon className="size-4.5 text-tone-success" />
          {t('card.issued', { domain: status.domain })}
        </p>
        <p className="text-xs text-muted-foreground">
          {t('card.issuedDetail', { issuer: status.issuer, date })}
        </p>
        {status.staging ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription>{t('card.stagingIssued')}</AlertDescription>
          </Alert>
        ) : (
          <a
            href={`https://${status.domain}`}
            rel="noreferrer"
            className={buttonVariants()}
          >
            {t('card.openDashboard')}
          </a>
        )}
      </div>
    )
  }
  return (
    <div className="space-y-3">
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
        <AlertTitle>{t('card.failedTitle')}</AlertTitle>
        <AlertDescription>
          <p>{t(`hint.${status.hint ?? 'unknown'}`)}</p>
          {status.error ? (
            <p className="mt-1 font-mono text-xs break-words">{status.error}</p>
          ) : null}
        </AlertDescription>
      </Alert>
      <p className="text-xs text-muted-foreground">{t('card.recheckHelp')}</p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" onClick={onRecheck} disabled={rechecking}>
          {rechecking ? t('card.rechecking') : t('card.recheck')}
        </Button>
        <Button type="button" variant="outline" onClick={onRetry}>
          {t('card.retry')}
        </Button>
      </div>
    </div>
  )
}

/** EnableHttpsCard is the one-click zero-DNS HTTPS flow for the dashboard. */
export function EnableHttpsCard({ compact = false }: { compact?: boolean }) {
  const { t } = useTranslation('https')
  const [expanded, setExpanded] = useState(false)
  const {
    data: status,
    refetch,
    isFetching,
  } = useQuery(httpsStatusQueryOptions())
  const enable = useEnableHttps()
  const username = useAuthUsername()
  const [retrying, setRetrying] = useState(false)
  useAutoSaveDashboardUrl(status)

  const showForm = status?.state === 'off' || retrying
  const initialEmail =
    status?.default_email ?? (username?.includes('@') ? username : '')
  if (
    compact &&
    !expanded &&
    status?.state === 'off' &&
    status.suggested_domain &&
    !retrying
  ) {
    return (
      <Card>
        <CardContent className="grid grid-cols-[1fr_auto] items-center gap-x-2 gap-y-0.5">
          <p className="flex min-w-0 items-center gap-2 text-sm font-medium">
            <LockGlassIcon aria-hidden="true" className="size-4.5 shrink-0" />
            <span className="truncate">{t('card.title')}</span>
          </p>
          <Button
            type="button"
            size="sm"
            aria-expanded={false}
            onClick={() => setExpanded(true)}
          >
            {t('card.setUp')}
          </Button>
          <p className="col-span-2 text-xs text-muted-foreground sm:col-span-1 sm:col-start-1">
            {t('card.compactHint')}
          </p>
        </CardContent>
      </Card>
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockGlassIcon className="size-4.5" />
          {t('card.title')}
        </CardTitle>
        {status?.state === 'off' && !status.suggested_domain ? (
          <CardDescription>{t('card.noPublicIp')}</CardDescription>
        ) : null}
      </CardHeader>
      <CardContent>
        {status && showForm && status.suggested_domain ? (
          <EnableForm
            suggestedDomain={status.suggested_domain}
            initialEmail={initialEmail}
            pending={enable.isPending}
            error={enable.error?.message}
            onSubmit={(email, staging) =>
              enable.mutate(
                { email, staging },
                { onSuccess: () => setRetrying(false) },
              )
            }
          />
        ) : null}
        {status && !showForm && status.state !== 'off' ? (
          <StatusBody
            status={status}
            preflight={enable.data?.preflight}
            onRetry={() => setRetrying(true)}
            onRecheck={() => void refetch()}
            rechecking={isFetching}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
