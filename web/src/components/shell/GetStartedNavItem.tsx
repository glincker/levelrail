import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { useIsRoot } from '../../hooks/useIsRoot'
import { firstAppProgress, showGuideEntryPoints } from '../../lib/setupGuide'
import { useSetupGuide } from '../../queries/setupGuide'

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
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          render={<Link to="/get-started" />}
          isActive={pathname === '/get-started'}
          tooltip={label}
        >
          <RocketGlassIcon size="1.4em" />
          <span>{t('nav.label')}</span>
          <span className="ml-auto text-xs tabular-nums text-muted-foreground">
            {progress ? (
              <>
                <span aria-hidden="true">
                  {progress.done}/{progress.total}
                </span>
                <span className="sr-only">{label}</span>
              </>
            ) : (
              `${guide?.ready_percent ?? 0}%`
            )}
          </span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
