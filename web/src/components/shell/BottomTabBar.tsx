import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BellGlassIcon,
  LayoutGridGlassIcon,
  RocketGlassIcon,
  SettingsGlassIcon,
} from '@thesvg/ui-react'

const TABS = [
  { id: 'apps', to: '/apps', Icon: LayoutGridGlassIcon },
  { id: 'alerts', to: '/alerts', Icon: BellGlassIcon },
  { id: 'deploys', to: '/deployments', Icon: RocketGlassIcon },
  { id: 'settings', to: '/settings', Icon: SettingsGlassIcon },
] as const

/** Phone-only primary navigation; the sidebar covers md and up. */
export function BottomTabBar() {
  const { t } = useTranslation('dashboard')
  return (
    <nav
      aria-label={t('phoneHome.tabs.label')}
      className="fixed inset-x-0 bottom-0 z-30 grid grid-cols-4 border-t border-border bg-background pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      {TABS.map(({ id, to, Icon }) => (
        <Link
          key={id}
          to={to}
          className="flex min-h-11 flex-col items-center justify-center gap-0.5 py-1 text-xs font-medium text-muted-foreground [&.active]:text-foreground"
          activeProps={{ 'aria-current': 'page', className: 'active' }}
        >
          <Icon size="1.4em" />
          {t(`phoneHome.tabs.${id}`)}
        </Link>
      ))}
    </nav>
  )
}
