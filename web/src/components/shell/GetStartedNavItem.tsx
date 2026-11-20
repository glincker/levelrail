import { Link, useRouterState } from '@tanstack/react-router'
import { useIsDemo } from '@/hooks/useDemo'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { NAV_ROW_CLASS } from './CollapsibleSection'
import { useIsRoot } from '../../hooks/useIsRoot'
import { ProgressRing } from '@/components/ui/progress-ring'
import { showGuideEntryPoints, stepCount } from '../../lib/setupGuide'
import { useSetupGuide } from '../../queries/setupGuide'

// Shown until the operator hides the guide or every step is settled.
export function GetStartedNavItem() {
  const { t } = useTranslation('getStarted')
  const isRoot = useIsRoot()
  const isDemo = useIsDemo()
  const { data: guide } = useSetupGuide()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  if (isDemo || !isRoot || !showGuideEntryPoints(guide)) return null
  const { done, total, percent } = stepCount(guide)
  const label = t('chip.steps', { done, total })
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
          <ProgressRing
            value={percent}
            label={label}
            display={`${done}/${total}`}
            className="ml-auto size-8 shrink-0"
          />
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
