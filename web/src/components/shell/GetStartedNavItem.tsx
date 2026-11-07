import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { NAV_ROW_CLASS } from './CollapsibleSection'
import { useIsRoot } from '../../hooks/useIsRoot'
import { firstAppProgress, showGuideEntryPoints } from '../../lib/setupGuide'
import { useSetupGuide } from '../../queries/setupGuide'

// Quarter steps keep the fill a Tailwind class instead of an inline width.
const FILL = ['w-0', 'w-1/4', 'w-1/2', 'w-3/4', 'w-full'] as const

// Shown until the operator hides the guide or every step is settled.
export function GetStartedNavItem() {
  const { t } = useTranslation('getStarted')
  const isRoot = useIsRoot()
  const { data: guide } = useSetupGuide()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  if (!isRoot || !showGuideEntryPoints(guide)) return null
  const progress = firstAppProgress(guide)
  const label = progress
    ? t('chip.firstApp', progress)
    : t('chip.label', { percent: guide?.ready_percent ?? 0 })
  const fraction = progress
    ? progress.total > 0
      ? progress.done / progress.total
      : 0
    : (guide?.ready_percent ?? 0) / 100
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          render={<Link to="/get-started" />}
          isActive={pathname === '/get-started'}
          tooltip={label}
          className={NAV_ROW_CLASS}
        >
          <RocketGlassIcon size="1.4em" />
          <span>{t('nav.label')}</span>
          <span
            aria-hidden="true"
            data-testid="get-started-progress"
            className="ml-auto h-1.5 w-12 shrink-0 overflow-hidden rounded-full bg-sidebar-foreground/20"
          >
            <span
              className={`block h-full rounded-full bg-state-active-bar transition-[width] duration-300 ease-out motion-reduce:transition-none ${FILL[Math.round(Math.min(1, Math.max(0, fraction)) * 4)]}`}
            />
          </span>
          <span className="sr-only">{label}</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
