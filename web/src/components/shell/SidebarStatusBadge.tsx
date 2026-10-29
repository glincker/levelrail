import { useTranslation } from 'react-i18next'
import {
  ArrowUpGlassIcon,
  CheckGlassIcon,
  CircleAlertGlassIcon,
  MinusGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { TONE, type Tone } from '@/components/kit/tone'
import { usePrefersReducedMotion } from '@/hooks/usePrefersReducedMotion'
import { cn } from '@/lib/utils'
import type { ShellStatus } from '@/lib/shellStatus'
import { useShellStatus } from './useShellStatus'

const STATUS_TONE: Record<ShellStatus, Tone> = {
  healthy: 'success',
  update: 'warning',
  degraded: 'warning',
  incident: 'danger',
  busy: 'info',
  unknown: 'neutral',
}

// Shape differs per state so color is never the only signal.
function StatusGlyph({ status }: { status: ShellStatus }) {
  const cls = 'size-2'
  switch (status) {
    case 'healthy':
      return <CheckGlassIcon size="1.2em" className={cls} aria-hidden />
    case 'update':
      return <ArrowUpGlassIcon size="1.2em" className={cls} aria-hidden />
    case 'degraded':
      return <CircleAlertGlassIcon size="1.2em" className={cls} aria-hidden />
    case 'incident':
      return <XGlassIcon size="1.2em" className={cls} aria-hidden />
    case 'unknown':
      return <MinusGlassIcon size="1.2em" className={cls} aria-hidden />
    default:
      return null
  }
}

export function SidebarStatusBadgeView({ status }: { status: ShellStatus }) {
  const { t } = useTranslation()
  const motion = !usePrefersReducedMotion()
  const label = t(`shell.logoStatus.${status}`)
  const tone = TONE[STATUS_TONE[status]]
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      data-status={status}
      className={cn(
        'absolute right-0 bottom-0 grid size-3.5 place-items-center rounded-full border-2 border-sidebar text-primary-foreground',
        status === 'busy' ? 'bg-sidebar' : tone.solid,
      )}
    >
      {status === 'busy' ? (
        <svg
          viewBox="0 0 16 16"
          aria-hidden="true"
          className={cn('size-3 text-tone-info', motion && 'animate-spin')}
        >
          <circle
            cx="8"
            cy="8"
            r="6"
            className="fill-none stroke-muted"
            strokeWidth="2.5"
          />
          <path
            d="M8 2a6 6 0 0 1 6 6"
            className="fill-none stroke-current"
            strokeWidth="2.5"
            strokeLinecap="round"
          />
        </svg>
      ) : (
        <StatusGlyph status={status} />
      )}
    </span>
  )
}

// An available update is shown once, on the version line; the logo badge stays a health signal.
export function SidebarStatusBadge() {
  const status = useShellStatus()
  return (
    <SidebarStatusBadgeView status={status === 'update' ? 'healthy' : status} />
  )
}
