import { useTranslation } from 'react-i18next'
import {
  RotateCwGlassIcon,
  ClockGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatRelativeTime } from '../../../lib/relativeTime'
import { expiryCountdown } from '../../../lib/gitCredential'
import type { GitCredentialKind } from '../../../types/gitConnections'

// Shared by the git provider panel and the Integrations card: how the
// credential stays valid, and when it stops. Never shows an expiry it was not given.
export function GitCredentialStatus({
  kind,
  expiresAt,
  lastRefreshAt,
  lastRefreshError,
  now,
  onReauthorize,
}: Readonly<{
  kind?: GitCredentialKind
  expiresAt?: string
  lastRefreshAt?: string
  lastRefreshError?: string
  now: number
  onReauthorize?: () => void
}>) {
  const { t } = useTranslation('gitConnect')
  if (!kind) return null
  const countdown = expiryCountdown(expiresAt, now)

  let expiry: React.ReactNode = null
  if (kind === 'static_token') {
    switch (countdown.kind) {
      case 'none':
        expiry = <Badge variant="muted">{t('credential.noExpiry')}</Badge>
        break
      case 'expired':
        expiry = (
          <Badge variant="destructive">
            <ClockGlassIcon aria-hidden="true" />
            {t('credential.expired', { count: countdown.days })}
          </Badge>
        )
        break
      default: {
        const soon = countdown.kind === 'hours' || countdown.count <= 7
        expiry = (
          <Badge variant={soon ? 'warning' : 'muted'}>
            <ClockGlassIcon aria-hidden="true" />
            {t(
              countdown.kind === 'hours'
                ? 'credential.expiresInHours'
                : 'credential.expiresInDays',
              { count: countdown.count },
            )}
          </Badge>
        )
      }
    }
  }

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-2">
        {kind === 'oauth_auto_renew' ? (
          <Badge variant="info">
            <RotateCwGlassIcon aria-hidden="true" />
            {t('credential.autoRenews')}
          </Badge>
        ) : null}
        {expiry}
        {kind === 'oauth_auto_renew' && lastRefreshAt && !lastRefreshError ? (
          <span className="text-xs text-muted-foreground">
            {t('credential.lastRefresh', {
              when: formatRelativeTime(lastRefreshAt, now),
            })}
          </span>
        ) : null}
      </div>
      {lastRefreshError ? (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-2 text-xs text-tone-danger"
        >
          <TriangleAlertGlassIcon
            className="size-4 shrink-0"
            aria-hidden="true"
          />
          <span className="font-medium">{t('credential.refreshFailing')}</span>
          <span className="break-words">{lastRefreshError}</span>
          {onReauthorize ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={onReauthorize}
            >
              {t('credential.reauthorize')}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
