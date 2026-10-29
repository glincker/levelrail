import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  GaugeGlassIcon,
  LoaderCircleGlassIcon,
  LockGlassIcon,
  WifiOffGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { StatusBadge } from '@/components/ui/status-badge'
import type { EndpointState } from '../../types/modelEndpoints'

const PRESENTATION: Record<
  EndpointState,
  { variant: 'success' | 'warning' | 'destructive' | 'muted'; icon: UiIcon }
> = {
  online: { variant: 'success', icon: CircleCheckGlassIcon },
  slow: { variant: 'warning', icon: GaugeGlassIcon },
  unreachable: { variant: 'destructive', icon: WifiOffGlassIcon },
  auth_failed: { variant: 'destructive', icon: LockGlassIcon },
  error: { variant: 'destructive', icon: CircleAlertGlassIcon },
  unknown: { variant: 'muted', icon: LoaderCircleGlassIcon },
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
