import * as React from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { groupForSlug } from '@/lib/appTabGroups'
import { useTranslation } from 'react-i18next'
import type { PaletteItem } from '../commandPaletteData'
import { APP_NAV_SECTIONS } from '../shell/navModel'

const APP_PATH = /^\/apps\/([^/]+)\//

/** Every tab of the app being viewed, ignoring any hiding, so the palette never loses one. */
export function useAppTabPaletteItems(): PaletteItem[] {
  const navigate = useNavigate()
  const { t } = useTranslation('common')
  const app = useRouterState({
    select: (s) => APP_PATH.exec(s.location.pathname)?.[1],
  })
  return React.useMemo(() => {
    if (!app) return []
    return APP_NAV_SECTIONS.flatMap((s) => s.items).map((item) => ({
      key: `app-tab-${app}-${item.slug}`,
      label: item.label,
      group: app,
      icon: item.icon,
      keywords: t(`appTabs.groups.${groupForSlug(item.slug)}`),
      run: () => void navigate({ to: item.to, params: { name: app } }),
    }))
  }, [app, navigate, t])
}
