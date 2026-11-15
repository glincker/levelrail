import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { formatRelativeTime } from '../../lib/relativeTime'
import {
  MANAGE_PATH,
  effectiveHealth,
  expiryInfo,
  healthState,
} from '../../lib/connections'
import type { Connection } from '../../types/connections'
import { ConnectionGlyph } from './ConnectionGlyph'

function ExpiryText({
  conn,
  now,
}: Readonly<{ conn: Connection; now: number }>) {
  const { t } = useTranslation('integrations')
  const info = expiryInfo(conn, now)
  if (info.days === null) return null
  const days = Math.abs(info.days)
  let text: string
  if (info.expired) text = t('expiry.expiredDaysAgo', { count: days })
  else if (info.days === 0) text = t('expiry.expiresToday')
  else text = t('expiry.expiresInDays', { count: days })
  return (
    <span
      className={
        info.expired || (conn.expiring_soon && !info.autoRenew)
          ? 'font-medium text-tone-danger'
          : undefined
      }
    >
      {text}
      {info.autoRenew && !info.expired ? ` (${t('expiry.autoRenew')})` : null}
    </span>
  )
}

export function ConnectionCard({
  conn,
  now,
}: Readonly<{ conn: Connection; now: number }>) {
  const { t } = useTranslation('integrations')
  const health = effectiveHealth(conn, now)
  const reconnect = health === 'needs_reconnect' || health === 'expired'
  const identity = conn.account ?? conn.host ?? conn.provider
  const meta = [
    conn.last_used_at
      ? t('card.lastUsed', { when: formatRelativeTime(conn.last_used_at, now) })
      : t('card.neverUsed'),
    t('card.bound', { count: conn.bound_count }),
  ]

  return (
    <li className="flex flex-col gap-3 py-3 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <ConnectionGlyph provider={conn.provider} kind={conn.kind} />
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="truncate text-sm font-medium text-foreground">
              {conn.label}
            </span>
            <StatusBadge
              form="dot"
              state={healthState(health)}
              label={t(`health.${health}`)}
            />
            <Badge variant="muted">
              {conn.scope_type === 'instance'
                ? t('scopes.instance')
                : t(`scopes.${conn.scope_type}`) +
                  (conn.scope_id ? `: ${conn.scope_id}` : '')}
            </Badge>
            {conn.is_default ? (
              <Badge variant="outline">{t('card.default')}</Badge>
            ) : null}
          </div>
          <p className="truncate text-xs text-muted-foreground">
            {identity}
            {' · '}
            {t(`authMethods.${conn.auth_method}`, {
              defaultValue: conn.auth_method,
            })}
          </p>
          <p className="flex flex-wrap gap-x-3 text-xs text-muted-foreground">
            {meta.map((m) => (
              <span key={m}>{m}</span>
            ))}
            <ExpiryText conn={conn} now={now} />
          </p>
          {conn.health_detail && health !== 'ok' ? (
            <p className="text-xs text-muted-foreground">
              {conn.health_detail}
            </p>
          ) : null}
        </div>
      </div>
      <Button
        variant={reconnect ? 'default' : 'outline'}
        size="sm"
        className="self-start sm:self-center"
        render={<Link to={MANAGE_PATH[conn.kind]} />}
        aria-label={t(reconnect ? 'card.reconnectNamed' : 'card.manageNamed', {
          name: conn.label,
        })}
      >
        {t(reconnect ? 'card.reconnect' : 'card.manage')}
        <ExternalLinkGlassIcon aria-hidden="true" />
      </Button>
    </li>
  )
}
