import { useEffect, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { rolesForConnection } from '../../lib/connectionBindings'
import { formatRelativeTime } from '../../lib/relativeTime'
import {
  MANAGE_PATH,
  effectiveHealth,
  expiryInfo,
  healthState,
} from '../../lib/connections'
import type { Connection } from '../../types/connections'
import { GitCredentialStatus } from '../settings/git/GitCredentialStatus'
import type { GitCredentialKind } from '../../types/gitConnections'
import { MetaRow, ObjectCard } from '../cards/ObjectCard'
import { BindConnectionDialog } from '../delivery/BindConnectionDialog'
import { UnbindConnectionDialog } from '../delivery/UnbindConnectionDialog'
import { ConnectionGlyph } from './ConnectionGlyph'

// The hub read model only knows the auth method, which is enough to tell
// a self-renewing OAuth grant from a pasted token on a git connection.
function credentialKind(conn: Connection): GitCredentialKind | undefined {
  if (conn.kind !== 'git') return undefined
  if (conn.auth_method === 'oauth') return 'oauth_auto_renew'
  if (conn.auth_method === 'token') return 'static_token'
  return undefined
}

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
  highlight = false,
}: Readonly<{ conn: Connection; now: number; highlight?: boolean }>) {
  const { t } = useTranslation('integrations')
  const health = effectiveHealth(conn, now)
  const reconnect = health === 'needs_reconnect' || health === 'expired'
  const identity = conn.account ?? conn.host ?? conn.provider
  const kind = credentialKind(conn)
  const reconnectRef = useRef<HTMLButtonElement>(null)
  const [dialog, setDialog] = useState<'bind' | 'unbind' | null>(null)
  const canBind = rolesForConnection(conn).length > 0

  // A deep link from an attention item lands on the card with Reconnect ready.
  useEffect(() => {
    if (highlight) reconnectRef.current?.focus({ preventScroll: true })
  }, [highlight])
  const scope =
    conn.scope_type === 'instance'
      ? t('scopes.instance')
      : t(`scopes.${conn.scope_type}`) +
        (conn.scope_id ? `: ${conn.scope_id}` : '')

  return (
    <li className="flex min-w-0">
      <ObjectCard
        id={`integration-${conn.ref}`}
        data-highlighted={highlight || undefined}
        highlight={highlight}
        icon={<ConnectionGlyph provider={conn.provider} kind={conn.kind} />}
        title={conn.label}
        status={
          <StatusBadge
            state={healthState(health)}
            label={t(`health.${health}`)}
          />
        }
        meta={[
          identity,
          t(`authMethods.${conn.auth_method}`, {
            defaultValue: conn.auth_method,
          }),
        ]}
        actions={
          <>
            {canBind ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setDialog('bind')}
                aria-label={t('card.bindNamed', { name: conn.label })}
              >
                {t('card.bind')}
              </Button>
            ) : null}
            {conn.bound_count > 0 ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setDialog('unbind')}
                aria-label={t('card.unbindNamed', { name: conn.label })}
              >
                {t('card.unbind')}
              </Button>
            ) : null}
            <Button
              ref={reconnectRef}
              variant={reconnect || highlight ? 'default' : 'outline'}
              size="sm"
              render={<Link to={MANAGE_PATH[conn.kind]} />}
              aria-label={t(
                reconnect ? 'card.reconnectNamed' : 'card.manageNamed',
                { name: conn.label },
              )}
            >
              {t(reconnect ? 'card.reconnect' : 'card.manage')}
              <ExternalLinkGlassIcon aria-hidden="true" />
            </Button>
          </>
        }
      >
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="muted">{scope}</Badge>
          {conn.is_default ? (
            <Badge variant="outline">{t('card.default')}</Badge>
          ) : null}
        </div>
        <MetaRow
          items={[
            conn.last_used_at
              ? t('card.lastUsed', {
                  when: formatRelativeTime(conn.last_used_at, now),
                })
              : t('card.neverUsed'),
            t('card.bound', { count: conn.bound_count }),
            kind || expiryInfo(conn, now).days === null ? null : (
              <ExpiryText conn={conn} now={now} />
            ),
          ]}
        />
        {kind ? (
          <GitCredentialStatus
            kind={kind}
            expiresAt={conn.expires_at}
            now={now}
          />
        ) : null}
        {conn.health_detail && health !== 'ok' ? (
          <p className="text-xs break-words text-muted-foreground">
            {conn.health_detail}
          </p>
        ) : null}
        {dialog === 'bind' ? (
          <BindConnectionDialog
            open
            onOpenChange={(o) => {
              if (!o) setDialog(null)
            }}
            connection={conn}
          />
        ) : null}
        {dialog === 'unbind' ? (
          <UnbindConnectionDialog
            open
            onOpenChange={(o) => {
              if (!o) setDialog(null)
            }}
            connectionRef={conn.ref}
            label={conn.label}
          />
        ) : null}
      </ObjectCard>
    </li>
  )
}
