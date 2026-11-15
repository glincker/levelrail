import type { ComponentType, SVGProps } from 'react'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleDotGlassIcon,
  CircleXGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import type { Tone } from '@/components/kit/tone'
import type { TimelineStatus } from '../types/appDelivery'

type IconType = ComponentType<SVGProps<SVGSVGElement>>

export interface StatusLook {
  icon: IconType
  tone: Tone
  spin?: boolean
}

const LOOKS: Record<TimelineStatus, StatusLook> = {
  succeeded: { icon: CircleCheckGlassIcon, tone: 'success' },
  failed: { icon: CircleXGlassIcon, tone: 'danger' },
  in_progress: { icon: LoaderCircleGlassIcon, tone: 'info', spin: true },
  pending: { icon: CircleDashedGlassIcon, tone: 'warning' },
  info: { icon: CircleDotGlassIcon, tone: 'neutral' },
}

const UNKNOWN: StatusLook = { icon: CircleAlertGlassIcon, tone: 'neutral' }

export function statusLook(status: string): StatusLook {
  return LOOKS[status as TimelineStatus] ?? UNKNOWN
}

export function statusKey(status: string): TimelineStatus | 'unknown' {
  return status in LOOKS ? (status as TimelineStatus) : 'unknown'
}
