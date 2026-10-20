import '../../../styles/upgradeMotion.css'
import type { ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useUpgradeStatus } from '../../../hooks/useUpgradeStatus'
import { usePrefersReducedMotion } from '../../../hooks/usePrefersReducedMotion'

const R = 9
const C = 2 * Math.PI * R

type IconType = ComponentType<{ className?: string }>

// Replaces the nav icon with a progress ring while an upgrade runs.
export function SidebarUpgradeIcon({ icon: Icon }: { icon: IconType }) {
  const s = useUpgradeStatus()
  const motion = !usePrefersReducedMotion()
  if (s.state !== 'running' && s.state !== 'reconnecting') return <Icon />
  const percent = s.view.percent
  const offset = C * (1 - (percent ?? 25) / 100)
  return (
    <span
      className="relative inline-grid size-4 shrink-0 place-items-center"
      aria-hidden
    >
      <svg
        viewBox="0 0 24 24"
        className={cn(
          'size-5 -rotate-90 text-primary',
          (percent === null || s.state === 'reconnecting') &&
            motion &&
            'animate-spin',
        )}
      >
        <circle
          cx="12"
          cy="12"
          r={R}
          className="fill-none stroke-muted"
          strokeWidth="3"
        />
        <circle
          cx="12"
          cy="12"
          r={R}
          className="fill-none stroke-current transition-[stroke-dashoffset] duration-500"
          strokeWidth="3"
          strokeLinecap="round"
          strokeDasharray={C}
          strokeDashoffset={offset}
        />
      </svg>
      <span className="absolute -right-1 -top-1 hidden size-2 rounded-full bg-primary group-data-[collapsible=icon]:block">
        {motion ? (
          <span className="upg-ring absolute inset-0 rounded-full bg-primary" />
        ) : null}
      </span>
    </span>
  )
}

export function SidebarUpgradeLabel() {
  const { t } = useTranslation('updates')
  const s = useUpgradeStatus()
  if (s.state !== 'running' && s.state !== 'reconnecting') return null
  const text =
    s.view.percent === null
      ? t('pipeline.sidebar.running')
      : `${s.view.percent}%`
  return (
    <span className="ml-auto rounded-full bg-primary/10 px-1.5 text-[10px] font-medium text-primary group-data-[collapsible=icon]:hidden">
      {text}
    </span>
  )
}
