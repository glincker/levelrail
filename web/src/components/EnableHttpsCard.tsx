import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  CheckCircleIcon,
  LockKeyIcon,
  SpinnerGapIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
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
      <div className="max-w-sm space-y-1.5">
        <Label htmlFor="enable-https-email">{t('card.emailLabel')}</Label>
        <Input
          id="enable-https-email"
          type="email"
          autoComplete="email"
          placeholder="you@example.com"
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
}: {
  status: HttpsStatus
  preflight?: HttpsStatus['preflight']
  onRetry: () => void
}) {
  const { t } = useTranslation('https')
  if (status.state === 'pending') {
    const closed = (preflight ?? []).filter((p) => !p.reachable)
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm" role="status">
          <SpinnerGapIcon className="size-4 animate-spin" />
          {t('card.pending', { domain: status.domain })}
        </p>
        {closed.length > 0 ? (
          <Alert>
            <WarningIcon />
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
          <CheckCircleIcon className="size-4 text-tone-success" weight="fill" />
          {t('card.issued', { domain: status.domain })}
        </p>
        <p className="text-xs text-muted-foreground">
          {t('card.issuedDetail', { issuer: status.issuer, date })}
        </p>
        {status.staging ? (
          <Alert>
            <WarningIcon />
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
        <WarningIcon />
        <AlertTitle>{t('card.failedTitle')}</AlertTitle>
        <AlertDescription>
          <p>{t(`hint.${status.hint ?? 'unknown'}`)}</p>
          {status.error ? (
            <p className="mt-1 font-mono text-xs break-words">{status.error}</p>
          ) : null}
        </AlertDescription>
      </Alert>
      <Button type="button" variant="outline" onClick={onRetry}>
        {t('card.retry')}
      </Button>
    </div>
  )
}

/** EnableHttpsCard is the one-click zero-DNS HTTPS flow for the dashboard. */
export function EnableHttpsCard() {
  const { t } = useTranslation('https')
  const { data: status } = useQuery(httpsStatusQueryOptions())
  const enable = useEnableHttps()
  const username = useAuthUsername()
  const [retrying, setRetrying] = useState(false)
  useAutoSaveDashboardUrl(status)

  const showForm = status?.state === 'off' || retrying
  const initialEmail = username?.includes('@') ? username : ''
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockKeyIcon className="size-4" />
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
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
