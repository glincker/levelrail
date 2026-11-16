import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleQuestionMarkGlassIcon,
  CircleXGlassIcon,
  KeyGlassIcon,
  PlusGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { ConnectionActions } from '../../ConnectionCard'
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

const CAP_STATE = {
  allowed: { Icon: CircleCheckGlassIcon, tone: 'text-tone-success' },
  needs_scope: { Icon: TriangleAlertGlassIcon, tone: 'text-tone-warning' },
  not_allowed: { Icon: CircleXGlassIcon, tone: 'text-tone-danger' },
  unknown: { Icon: CircleQuestionMarkGlassIcon, tone: 'text-muted-foreground' },
} as const

// Icon, tone and words each carry the state, so none depends on colour alone.
function CapabilityState({
  state,
  inferred,
}: Readonly<{ state: CapabilityState; inferred: boolean }>) {
  const { t } = useTranslation('gitConnect')
  let key: keyof typeof CAP_STATE
  let label: string
  switch (state.kind) {
    case 'allowed':
      key = 'allowed'
      label = t('access.allowed')
      break
    case 'needs_scope':
      key = 'needs_scope'
      label = t('access.needsScope', { scope: state.scope })
      break
    case 'not_allowed':
      key = 'not_allowed'
      label = t('access.notAllowed')
      break
    default:
      key = 'unknown'
      label = t('access.notReported')
  }
  const { Icon, tone } = CAP_STATE[key]
  return (
    <span className="flex shrink-0 flex-wrap items-center justify-end gap-x-2 gap-y-1">
      {inferred && key !== 'unknown' ? (
        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
          <CircleDashedGlassIcon className="size-3.5" aria-hidden="true" />
          {t('access.inferred')}
        </span>
      ) : null}
      <span
        className={cn(
          'inline-flex items-center gap-1.5 text-[13px] font-medium',
          tone,
        )}
      >
        <Icon className="size-4 shrink-0" aria-hidden="true" />
        {label}
      </span>
    </span>
  )
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
      <CardContent className="space-y-6">
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
        <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
          <section
            aria-labelledby={`git-access-${connection.id}`}
            className="space-y-2"
          >
            <h3
              id={`git-access-${connection.id}`}
              className="font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-banner) leading-(--glinr-type-leading-banner) font-(weight:--glinr-type-heading-weight)"
            >
              {t('access.matrix')}
            </h3>
            {granted === null ? (
              <p className="text-xs text-muted-foreground">
                {t('access.scopesUnknown')}
              </p>
            ) : null}
            <ul className="divide-y divide-border">
              {CAPABILITY_IDS.map((id) => (
                <li
                  key={id}
                  className="flex min-h-(--glinr-row-h) flex-wrap items-center justify-between gap-x-3 gap-y-1 py-2"
                >
                  <span className="min-w-0 text-sm">{t(`cap.${id}`)}</span>
                  <CapabilityState {...capabilityFor(id)} />
                </li>
              ))}
            </ul>
          </section>
          <div className="space-y-4 text-sm lg:border-l lg:border-border lg:pl-6">
            {info?.live_check_error ? (
              <p role="status" className="text-xs text-muted-foreground">
                {info.live_check_error}
              </p>
            ) : null}
            <dl className="space-y-4">
              <Fact label={t('access.scopes')}>
                {granted && granted.length > 0 ? (
                  <ul className="flex flex-wrap gap-1.5">
                    {granted.map((s) => (
                      <li key={s}>
                        <code className="rounded-(--glinr-radius-xs) bg-muted px-1.5 py-0.5 text-xs">
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
        </div>
        <ConnectionActions>
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
        </ConnectionActions>
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
