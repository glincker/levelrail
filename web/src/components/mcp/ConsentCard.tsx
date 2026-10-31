import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  BotGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { ApiError } from '../../lib/apiError'
import { devModeQueryOptions } from '../../queries/devMode'
import {
  useDecideMcpConsent,
  useMcpConsent,
  type McpConsent,
} from '../../queries/mcpOauth'

const SENSITIVE_SCOPE = 'mcp:read:sensitive'
const SCOPE_LABEL_KEYS: Record<string, 'scopes.read' | 'scopes.sensitive'> = {
  'mcp:read': 'scopes.read',
  [SENSITIVE_SCOPE]: 'scopes.sensitive',
}
const MCP_FEATURE = 'mcp-http'
const SECONDS_PER_MINUTE = 60
const SECONDS_PER_HOUR = 3600
const SECONDS_PER_DAY = 86400

function useDuration() {
  const { t } = useTranslation('mcpConsent')
  return (seconds: number) => {
    if (seconds >= SECONDS_PER_DAY) {
      return t('duration.days', {
        count: Math.round(seconds / SECONDS_PER_DAY),
      })
    }
    if (seconds >= SECONDS_PER_HOUR) {
      return t('duration.hours', {
        count: Math.round(seconds / SECONDS_PER_HOUR),
      })
    }
    return t('duration.minutes', {
      count: Math.max(1, Math.round(seconds / SECONDS_PER_MINUTE)),
    })
  }
}

const CARD_CLASS =
  'space-y-4 rounded-xl border border-border bg-card p-5 shadow-sm'

// Mirrors the loaded card's rows so the page does not jump when data lands.
function ConsentSkeleton() {
  return (
    <div className={CARD_CLASS} aria-hidden="true">
      <div className="flex items-start gap-3">
        <Skeleton className="size-8 shrink-0" />
        <div className="min-w-0 flex-1 space-y-1.5">
          <Skeleton className="h-6 w-3/4" />
          <Skeleton className="h-5 w-1/2" />
        </div>
      </div>
      <Skeleton className="h-4 w-2/3" />
      <Skeleton className="h-20 w-full" />
      <div className="space-y-1">
        <Skeleton className="h-5 w-full" />
        <Skeleton className="h-5 w-full" />
        <Skeleton className="h-5 w-full" />
        <Skeleton className="h-5 w-full" />
      </div>
      <div className="flex justify-end gap-2">
        <Skeleton className="h-8 w-20" />
        <Skeleton className="h-8 w-24" />
      </div>
    </div>
  )
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto flex w-full max-w-lg flex-col gap-3 py-8">
      {children}
    </div>
  )
}

export function ConsentCard({ requestId }: { requestId: string | undefined }) {
  const { t } = useTranslation('mcpConsent')
  const { data: dev } = useQuery(devModeQueryOptions())
  const consent = useMcpConsent(requestId ?? '')

  if (!requestId) {
    return (
      <Shell>
        <Banner variant="critical" title={t('errors.missing')} />
      </Shell>
    )
  }
  if (dev && !dev.experimental.includes(MCP_FEATURE)) {
    return (
      <Shell>
        <Banner variant="warning" title={t('errors.featureOff')} />
      </Shell>
    )
  }
  if (consent.isPending) {
    return (
      <Shell>
        <ConsentSkeleton />
      </Shell>
    )
  }
  if (consent.error) {
    const status =
      consent.error instanceof ApiError ? consent.error.status : undefined
    const key =
      status === 404
        ? 'errors.gone'
        : status === 501
          ? 'errors.unavailable'
          : 'errors.load'
    return (
      <Shell>
        <Banner variant="critical" title={t(key)} />
      </Shell>
    )
  }
  return <ConsentBody requestId={requestId} consent={consent.data} />
}

function ConsentBody({
  requestId,
  consent,
}: {
  requestId: string
  consent: McpConsent
}) {
  const { t } = useTranslation('mcpConsent')
  const duration = useDuration()
  const decide = useDecideMcpConsent(requestId)
  const [done, setDone] = useState(false)
  const username = useAuthUsername()
  const host = window.location.host
  const grantable = consent.grantable_scopes
  const sensitive = grantable.includes(SENSITIVE_SCOPE)
  const trimmed = consent.requested_scopes.length > grantable.length

  function submit(approve: boolean) {
    decide.mutate(
      { approve, scopes: grantable },
      {
        onSuccess: (res) => {
          setDone(true)
          // Script-capable schemes are never a valid OAuth redirect target.
          if (!/^(javascript|data|vbscript):/i.test(res.redirect_to.trim())) {
            window.location.assign(res.redirect_to)
          }
        },
      },
    )
  }

  const busy = decide.isPending || done

  return (
    <Shell>
      <section aria-labelledby="mcp-consent-title" className={CARD_CLASS}>
        <header className="flex items-start gap-3">
          <BotGlassIcon className="mt-0.5 size-8 shrink-0" aria-hidden="true" />
          <div className="min-w-0">
            <h1 id="mcp-consent-title" className="text-base font-semibold">
              {t('title', { client: consent.client_name })}
            </h1>
            <p className="text-sm text-muted-foreground">
              {t('redirectsTo')}{' '}
              <span className="font-mono break-all text-foreground">
                {consent.redirect_host}
              </span>
            </p>
          </div>
        </header>

        <p
          className="text-sm text-muted-foreground"
          data-testid="consent-account"
        >
          {username
            ? t('signedInAs', { user: username, host })
            : t('instanceOnly', { host })}
        </p>

        <div
          className={
            sensitive
              ? 'space-y-1 rounded-lg border border-tone-warning bg-tone-warning-soft p-3 text-sm'
              : 'space-y-1 rounded-lg border border-border p-3 text-sm'
          }
        >
          <p className="flex items-center gap-2 font-medium">
            {sensitive ? (
              <TriangleAlertGlassIcon className="size-5" aria-hidden="true" />
            ) : (
              <ShieldCheckGlassIcon className="size-5" aria-hidden="true" />
            )}
            {grantable.length === 0
              ? t('access.none')
              : sensitive
                ? t('access.sensitive')
                : t('access.read')}
          </p>
          {grantable.length > 0 ? (
            <ul aria-label={t('scopes.heading')} className="space-y-0.5 pl-7">
              {grantable.map((scope) => (
                <li key={scope} data-testid="consent-scope">
                  {SCOPE_LABEL_KEYS[scope] ? (
                    t(SCOPE_LABEL_KEYS[scope])
                  ) : (
                    <span className="font-mono">{scope}</span>
                  )}
                </li>
              ))}
            </ul>
          ) : null}
          {trimmed && grantable.length > 0 ? (
            <p className="text-muted-foreground">{t('access.trimmed')}</p>
          ) : null}
        </div>

        <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
          <li>{t('facts.never')}</li>
          <li>{t('facts.permissions')}</li>
          <li>
            {t('facts.expiry', {
              access: duration(consent.access_ttl_seconds),
              refresh: duration(consent.refresh_ttl_seconds),
            })}
          </li>
          <li>{t('facts.revoke')}</li>
        </ul>

        {decide.error ? (
          <Banner variant="critical" title={t('errors.decide')}>
            {decide.error.message}
          </Banner>
        ) : null}

        <div className="flex flex-wrap justify-end gap-2">
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => submit(false)}
          >
            {t('deny')}
          </Button>
          <Button
            disabled={busy || grantable.length === 0}
            onClick={() => submit(true)}
          >
            {decide.isPending
              ? t('working')
              : sensitive
                ? t('approveSensitive')
                : t('approve')}
          </Button>
        </div>
      </section>
    </Shell>
  )
}
