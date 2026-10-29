import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { HelpLink } from '../HelpLink'
import {
  ENFORCEMENT_DOCS_PATH,
  enforcementProxyKey,
  enforcementReasonKey,
  isEnforced,
  isProxyMode,
} from '../../lib/domainEnforcement'
import type { TrafficControl } from '../../lib/domainEnforcement'
import { useDomainEnforcement } from '../../hooks/useDomainEnforcement'

// Renders children (the "on" badge) only when visitors really get the
// control; anything else shows as saved, never as active.
export function EnforcedBadge({
  domain,
  control,
  children,
}: {
  domain: string
  control: TrafficControl
  children: React.ReactNode
}) {
  const { t } = useTranslation('domains')
  const enforcement = useDomainEnforcement(domain)
  if (isEnforced(enforcement, control)) return <>{children}</>
  return (
    <Badge variant="warning" className="shrink-0">
      <TriangleAlertGlassIcon
        className="size-3.5 text-tone-warning"
        aria-hidden="true"
      />
      {t('enforcement.savedNotEnforced')}
    </Badge>
  )
}

// Explains why a saved control is not enforced, naming the proxy.
export function EnforcementNote({
  domain,
  control,
  active,
}: {
  domain: string
  control: TrafficControl
  active: boolean
}) {
  const { t } = useTranslation('domains')
  const enforcement = useDomainEnforcement(domain)
  if (!active || !enforcement || isEnforced(enforcement, control)) return null
  const proxy = t(enforcementProxyKey[enforcement.mode])
  const reason = enforcement.reason ?? 'not_routed'
  return (
    <div
      role="status"
      className="mt-2 space-y-1 rounded-md border border-tone-warning-border bg-tone-warning-soft p-2 text-xs text-foreground"
    >
      <p>{t(enforcementReasonKey[reason], { proxy, domain })}</p>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        {isProxyMode(enforcement.mode) ? (
          <Link to="/network/proxy" className="underline">
            {t('enforcement.proxyPage')}
          </Link>
        ) : null}
        <HelpLink
          path={ENFORCEMENT_DOCS_PATH}
          label={t('enforcement.docs')}
          variant="inline"
        />
      </div>
    </div>
  )
}
