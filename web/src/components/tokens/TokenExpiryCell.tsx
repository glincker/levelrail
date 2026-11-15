import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { RelativeTime } from '../kit/RelativeTime'
import { hoursUntil, tokenExpiry } from '../../lib/tokenExpiry'
import type { TokenResource } from '../../types/token'

const BADGE_VARIANT = {
  ok: 'success',
  soon: 'warning',
  expired: 'destructive',
} as const

// Expiry badge plus the time left, and a note while a rotated token is in its grace period.
export function TokenExpiryCell({
  token,
  now,
  soonDays,
}: {
  token: TokenResource
  now: number
  soonDays: number
}) {
  const { t } = useTranslation('tokenExpiry')
  const { t: ts } = useTranslation('settings')
  const expiry = tokenExpiry(token.expires_at, now, soonDays)
  const revokeHours = hoursUntil(token.revokes_at, now)
  return (
    <div className="flex flex-col items-start gap-1">
      {expiry.state === 'none' ? (
        <span className="text-muted-foreground">{ts('tokens.never')}</span>
      ) : (
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant={BADGE_VARIANT[expiry.state]}>
            {t(`badge.${expiry.state}`)}
          </Badge>
          <span className="text-muted-foreground">
            {expiry.state === 'expired' && token.expires_at ? (
              <RelativeTime at={token.expires_at} />
            ) : (
              t(`left.${expiry.unit}`, { count: expiry.count })
            )}
          </span>
        </div>
      )}
      {revokeHours > 0 ? (
        <span className="text-xs text-tone-warning">
          {t('revokesIn', { count: revokeHours })}
        </span>
      ) : null}
    </div>
  )
}
