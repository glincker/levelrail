import {
  ClockGlassIcon,
  CpuGlassIcon,
  UnplugGlassIcon,
  RotateCcwGlassIcon,
  ShieldAlertGlassIcon,
} from '@thesvg/ui-react'
import type { KindWithIcon } from '@/lib/attentionKindIcon'

const ICONS = {
  host_pressure: CpuGlassIcon,
  deploy_interrupted: RotateCcwGlassIcon,
  unclean_restart: ShieldAlertGlassIcon,
  git_connection_reauth: UnplugGlassIcon,
  git_token_expiring: ClockGlassIcon,
  token_grace_sweep_failing: ShieldAlertGlassIcon,
} as const

export function AttentionKindIcon({
  kind,
  className,
}: {
  kind: KindWithIcon
  className: string
}) {
  const Icon = ICONS[kind]
  return <Icon className={className} aria-hidden="true" />
}
