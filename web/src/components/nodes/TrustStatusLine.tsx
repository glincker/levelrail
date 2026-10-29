import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  ShieldAlertGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import type { NodeTrustResource } from '../../types/nodeTrust'

const TONE: Record<string, string> = {
  verified: 'text-tone-success',
  rejected: 'text-tone-danger',
  revoked: 'text-tone-danger',
  expired: 'text-tone-danger',
  idle: 'text-muted-foreground',
  no_activity: 'text-muted-foreground',
}

// The headline is computed server side from real recent events; this only
// words it. It never claims a state the API did not report.
export function TrustStatusLine({ trust }: { trust: NodeTrustResource }) {
  const { t } = useTranslation('nodes')
  const { state, reason } = trust.status
  const Icon =
    state === 'verified'
      ? ShieldCheckGlassIcon
      : state === 'revoked'
        ? BanGlassIcon
        : state === 'rejected' || state === 'expired'
          ? TriangleAlertGlassIcon
          : ShieldAlertGlassIcon
  return (
    <p
      className={`flex items-start gap-2 text-sm font-medium ${TONE[state] ?? ''}`}
      role="status"
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span>
        {t(`trust.status.${state}`, {
          reason: reason
            ? t(`trust.reasons.${reason}`, { defaultValue: reason })
            : '',
        })}
      </span>
    </p>
  )
}
