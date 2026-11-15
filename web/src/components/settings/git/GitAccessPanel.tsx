import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  KeyGlassIcon,
  PlusGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  CAPABILITY_IDS,
  type CapabilityId,
  capabilityState,
  parseScopes,
  type CapabilityState,
} from '../../../lib/gitAccess'
import type { GitConnectProvider } from '../../../lib/gitHost'
import { useGitConnectionAccess } from '../../../queries/gitConnections'
import type { GitProviderConnection } from '../../../types/gitConnections'
import { GitConnectionRow } from '../GitConnectionRow'
import { GitCredentialStatus } from './GitCredentialStatus'
import { RotateGitTokenDialog } from './RotateGitTokenDialog'

function StateBadge({
  state,
  inferred,
}: Readonly<{ state: CapabilityState; inferred: boolean }>) {
  const { t } = useTranslation('gitConnect')
  const mark = inferred ? (
    <span className="text-xs text-muted-foreground">
      {t('access.inferred')}
    </span>
  ) : null
  switch (state.kind) {
    case 'allowed':
      return (
        <span className="flex items-center gap-2">
          {mark}
          <Badge variant="success">
            <CircleCheckGlassIcon aria-hidden="true" />
            {t('access.allowed')}
          </Badge>
        </span>
      )
    case 'needs_scope':
      return (
        <span className="flex items-center gap-2">
          {mark}
          <Badge variant="warning">
            <TriangleAlertGlassIcon aria-hidden="true" />
            {t('access.needsScope', { scope: state.scope })}
          </Badge>
        </span>
      )
    case 'not_allowed':
      return (
        <span className="flex items-center gap-2">
          {mark}
          <Badge variant="destructive">
            <CircleXGlassIcon aria-hidden="true" />
            {t('access.notAllowed')}
          </Badge>
        </span>
      )
    default:
      return <Badge variant="muted">{t('access.notReported')}</Badge>
  }
}

function DisabledAction({
  icon,
  label,
  reason,
}: Readonly<{ icon: React.ReactNode; label: string; reason: string }>) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span tabIndex={0} className="inline-flex">
            <Button type="button" size="sm" variant="outline" disabled>
              {icon}
              {label}
            </Button>
          </span>
        }
      />
      <TooltipContent>{reason}</TooltipContent>
    </Tooltip>
  )
}

// One connected account in full: the account row, what its scopes allow, what
// it can see and when it expires. Fields the API does not return say so.
export function GitAccessPanel({
  provider,
  connection,
  canSetDefault,
  onAddAnother,
}: Readonly<{
  provider: GitConnectProvider
  connection: GitProviderConnection
  canSetDefault: boolean
  onAddAnother: () => void
}>) {
  const { t } = useTranslation('gitConnect')
  const { data: info } = useGitConnectionAccess(connection.id)
  const granted = info?.scopes_known
    ? info.scopes
    : parseScopes(connection.scopes)
  const [now] = useState(() => Date.now())
  const [rotateOpen, setRotateOpen] = useState(false)
  const isToken = connection.auth_method === 'token'
  const canRotate = connection.can_rotate === true
  const none = t('access.notReported')
  const capabilityFor = (
    id: CapabilityId,
  ): { state: CapabilityState; inferred: boolean } => {
    const live = info?.capabilities?.find((c) => c.id === id)
    if (live?.state === 'needs_scope') {
      return {
        state: { kind: 'needs_scope', scope: live.needs_scope ?? '' },
        inferred: !live.verified,
      }
    }
    if (live) {
      return {
        state: { kind: live.state === 'allowed' ? 'allowed' : 'not_allowed' },
        inferred: !live.verified,
      }
    }
    return { state: capabilityState(provider, id, granted), inferred: false }
  }

  return (
    <Card>
      <CardContent className="space-y-5">
        <ul data-slot="grouped-list">
          <GitConnectionRow
            connection={connection}
            canSetDefault={canSetDefault}
          />
        </ul>
        <GitCredentialStatus
          kind={connection.credential_kind}
          expiresAt={connection.expires_at}
          lastRefreshAt={connection.last_refresh_at}
          lastRefreshError={connection.last_refresh_error}
          now={now}
          onReauthorize={
            connection.authorize_path
              ? () => window.location.assign(connection.authorize_path)
              : undefined
          }
        />
        <div className="grid gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
          <section aria-label={t('access.matrix')} className="space-y-2">
            <h3 className="text-sm font-medium">{t('access.matrix')}</h3>
            {granted === null ? (
              <p className="text-xs text-muted-foreground">
                {t('access.scopesUnknown')}
              </p>
            ) : null}
            <ul className="divide-y divide-border rounded-md border border-border">
              {CAPABILITY_IDS.map((id) => (
                <li
                  key={id}
                  className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                >
                  <span className="text-sm">{t(`cap.${id}`)}</span>
                  <StateBadge {...capabilityFor(id)} />
                </li>
              ))}
            </ul>
          </section>
          <dl className="space-y-3 text-sm">
            {info?.live_check_error ? (
              <p role="status" className="text-xs text-muted-foreground">
                {info.live_check_error}
              </p>
            ) : null}
            <Fact label={t('access.scopes')}>
              {granted && granted.length > 0 ? (
                <ul className="flex flex-wrap gap-1.5">
                  {granted.map((s) => (
                    <li key={s}>
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                        {s}
                      </code>
                    </li>
                  ))}
                </ul>
              ) : (
                none
              )}
            </Fact>
            <Fact label={t('access.sees')}>
              {info
                ? [
                    info.visible_accounts.names.join(', '),
                    t('access.projects', {
                      count: info.visible_accounts.total,
                    }),
                  ]
                    .filter(Boolean)
                    .join(' · ')
                : none}
            </Fact>
            <Fact label={t('access.expires')}>
              {connection.expires_at || info?.expires_at
                ? new Date(
                    connection.expires_at ?? info?.expires_at ?? '',
                  ).toLocaleDateString()
                : t('credential.noExpiry')}
            </Fact>
            <Fact label={t('access.lastUsed')}>
              {info?.last_used_at
                ? new Date(info.last_used_at).toLocaleString()
                : none}
            </Fact>
          </dl>
        </div>
        <div className="flex flex-wrap gap-2 border-t border-border pt-4">
          <DisabledAction
            icon={<PlusGlassIcon className="size-5" aria-hidden="true" />}
            label={t('access.addScope')}
            reason={t('access.addScopeMissing')}
          />
          {canRotate ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setRotateOpen(true)}
            >
              <KeyGlassIcon className="size-5" aria-hidden="true" />
              {t('access.rotate')}
            </Button>
          ) : isToken || connection.credential_kind ? (
            <DisabledAction
              icon={<KeyGlassIcon className="size-5" aria-hidden="true" />}
              label={t('access.rotate')}
              reason={t(
                connection.credential_kind === 'oauth_auto_renew'
                  ? 'access.rotateOauth'
                  : 'access.rotateUnsupported',
              )}
            />
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onAddAnother}
          >
            <PlusGlassIcon className="size-5" aria-hidden="true" />
            {t('access.addAnother')}
          </Button>
        </div>
        {canRotate ? (
          <RotateGitTokenDialog
            connection={connection}
            open={rotateOpen}
            onOpenChange={setRotateOpen}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}

function Fact({
  label,
  children,
}: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <div className="space-y-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="break-words">{children}</dd>
    </div>
  )
}
