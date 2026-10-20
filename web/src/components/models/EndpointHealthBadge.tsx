import { useTranslation } from 'react-i18next'
import type { Icon } from '@phosphor-icons/react'
import {
  CheckCircleIcon,
  CircleNotchIcon,
  GaugeIcon,
  LockKeyIcon,
  WarningCircleIcon,
  WifiSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { StatusBadge } from '@/components/ui/status-badge'
import type { EndpointState } from '../../types/modelEndpoints'

const PRESENTATION: Record<
  EndpointState,
  { variant: 'success' | 'warning' | 'destructive' | 'muted'; icon: Icon }
> = {
  online: { variant: 'success', icon: CheckCircleIcon },
  slow: { variant: 'warning', icon: GaugeIcon },
  unreachable: { variant: 'destructive', icon: WifiSlashIcon },
  auth_failed: { variant: 'destructive', icon: LockKeyIcon },
  error: { variant: 'destructive', icon: WarningCircleIcon },
  unknown: { variant: 'muted', icon: CircleNotchIcon },
}

export function EndpointHealthBadge({
  state,
  latencyMs,
}: {
  state: EndpointState
  latencyMs?: number
}) {
  const { t } = useTranslation('models')
  const p = PRESENTATION[state]
  const label =
    latencyMs !== undefined && (state === 'online' || state === 'slow')
      ? t('endpoint.stateWithLatency', {
          state: t(`endpoint.state.${state}`),
          ms: latencyMs,
        })
      : t(`endpoint.state.${state}`)
  return <StatusBadge variant={p.variant} label={label} icon={p.icon} />
}
