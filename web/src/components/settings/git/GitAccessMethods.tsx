import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  KeyGlassIcon,
  PlugGlassIcon,
  RocketGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import type { UiIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import type { StatusState } from '@/components/kit/tone'
import { CardGrid, ObjectCard } from '../../cards/ObjectCard'
import {
  GIT_ACCESS_METHODS,
  methodStatus,
  recommendedMethod,
  type AccessMethodId,
} from '../../../lib/gitAccess'
import type { GitConnectProvider } from '../../../lib/gitHost'
import type { GitAccessInfo } from '../../../types/gitAccess'
import type { GitProviderConnection } from '../../../types/gitConnections'

const ICON: Record<AccessMethodId, UiIcon> = {
  oauth: PlugGlassIcon,
  token: KeyGlassIcon,
  deploy_token: ServerGlassIcon,
  ci_job: RocketGlassIcon,
}

// Same status language as the Integrations cards: shape plus tone plus text.
const STATE: Record<
  'connected' | 'not_connected' | 'needs_attention' | 'expires_soon',
  StatusState
> = {
  connected: 'healthy',
  not_connected: 'pending',
  needs_attention: 'failed',
  expires_soon: 'degraded',
}

// The ways in for one provider, each with what it unlocks and one action.
export function GitAccessMethods({
  provider,
  connections,
  info,
  origin = window.location.origin,
  onOAuth,
  onToken,
}: Readonly<{
  provider: GitConnectProvider
  connections: GitProviderConnection[]
  info?: GitAccessInfo
  origin?: string
  onOAuth: () => void
  onToken: () => void
}>) {
  const { t } = useTranslation('gitConnect')
  const methods = GIT_ACCESS_METHODS[provider]
  const recommended = recommendedMethod(provider, origin)
  const [now] = useState(() => Date.now())

  return (
    <section aria-labelledby="git-access-methods" className="space-y-4">
      <h2
        id="git-access-methods"
        className="font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)"
      >
        {t('methods.title')}
      </h2>
      <CardGrid min="sm">
        {methods.map((method) => {
          const status = methodStatus(method, connections, info, now)
          const Icon = ICON[method]
          return (
            <li key={method} className="flex min-w-0">
              <ObjectCard
                icon={<Icon aria-hidden="true" />}
                title={t(`methods.${method}.name`)}
                badges={
                  method === recommended ? (
                    <Badge variant="accent">{t('methods.recommended')}</Badge>
                  ) : null
                }
                status={
                  status === 'separate' ? null : (
                    <StatusBadge
                      state={STATE[status]}
                      label={t(`methods.status.${status}`)}
                    />
                  )
                }
                actions={
                  <MethodAction
                    method={method}
                    primary={method === recommended}
                    onOAuth={onOAuth}
                    onToken={onToken}
                    label={t(`methods.${method}.action`)}
                  />
                }
              >
                <p className="text-[13px] text-muted-foreground">
                  {t(`methods.${method}.unlocks`)}
                </p>
              </ObjectCard>
            </li>
          )
        })}
      </CardGrid>
      {provider === 'gitlab' ? (
        <p className="text-xs text-muted-foreground">
          {t('methods.selfManaged')}
        </p>
      ) : null}
    </section>
  )
}

function MethodAction({
  method,
  primary,
  onOAuth,
  onToken,
  label,
}: Readonly<{
  method: AccessMethodId
  primary: boolean
  onOAuth: () => void
  onToken: () => void
  label: string
}>) {
  const variant = primary ? 'default' : 'outline'
  if (method === 'deploy_token') {
    return (
      <Button
        size="sm"
        variant={variant}
        render={<Link to="/settings/registry-credentials" />}
      >
        {label}
      </Button>
    )
  }
  if (method === 'ci_job') {
    return (
      <Button size="sm" variant={variant} render={<Link to="/pipelines" />}>
        {label}
      </Button>
    )
  }
  return (
    <Button
      type="button"
      size="sm"
      variant={variant}
      onClick={method === 'oauth' ? onOAuth : onToken}
    >
      {label}
    </Button>
  )
}
