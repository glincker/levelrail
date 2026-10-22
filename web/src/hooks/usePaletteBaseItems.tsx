import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import {
  KeyboardIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { usePageActions } from '@/lib/pageActions'
import { openCliModal } from '@/lib/cliModalStore'
import { useTheme, type Theme } from '../components/ThemeProvider'
import {
  ROUTE_ENTRIES,
  routeEntryItem,
  THEME_ACTION,
  type PaletteItem,
} from '../components/commandPaletteData'
import {
  domainFromPath,
  useTrafficPaletteItems,
} from '../components/trafficPaletteItems'
import { useResourcePaletteItems } from '../components/resourcePaletteItems'
import { useAppTabPaletteItems } from '../components/apps/useAppTabPaletteItems'
import { useExperimentalFeatures } from './useExperimental'

const NEXT_THEME: Record<Theme, Theme> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
}
const NO_DOMAINS: readonly never[] = []

// Client-only entries (routes, page actions, theme); entities come from the search service.
export function usePaletteBaseItems(
  onShowShortcuts?: () => void,
): PaletteItem[] {
  const navigate = useNavigate()
  const { t } = useTranslation('palette')
  const { t: tCommon } = useTranslation()
  const { t: tCli } = useTranslation('signIn', { useSuspense: false })
  const { theme, setTheme } = useTheme()
  const pageActions = usePageActions()
  const experimental = useExperimentalFeatures()
  const currentDomain = useRouterState({
    select: (st) => domainFromPath(st.location.pathname),
  })
  const traffic = useTrafficPaletteItems({
    domains: NO_DOMAINS,
    query: '',
    currentDomain,
  })
  const resourceItems = useResourcePaletteItems()
  const appTabItems = useAppTabPaletteItems()

  return React.useMemo<PaletteItem[]>(() => {
    const go =
      (
        to: string,
        params?: Record<string, string>,
        search?: Record<string, string>,
      ) =>
      () =>
        void navigate(search ? { to, params, search } : { to, params })
    const items: PaletteItem[] = ROUTE_ENTRIES.map((e) =>
      routeEntryItem(
        e,
        experimental,
        go,
        tCommon('shell.locked.paletteSubtitle'),
      ),
    )
    for (const a of pageActions) items.push({ ...a, group: 'Actions' })
    items.push({ ...THEME_ACTION, run: () => setTheme(NEXT_THEME[theme]) })
    if (onShowShortcuts) {
      items.push({
        key: 'action-shortcuts',
        label: t('keyboardShortcuts'),
        keywords: t('keyboardShortcutsKeywords'),
        group: 'Actions',
        icon: <KeyboardIcon />,
        run: onShowShortcuts,
      })
    }
    items.push({
      key: 'action-cli-setup',
      label: tCli('cliModal.openAction'),
      keywords: tCli('cliModal.openKeywords'),
      group: 'Actions',
      icon: <TerminalWindowIcon />,
      run: openCliModal,
    })
    items.push(...traffic.base, ...resourceItems, ...appTabItems)
    return items
  }, [
    navigate,
    setTheme,
    theme,
    traffic.base,
    resourceItems,
    appTabItems,
    onShowShortcuts,
    pageActions,
    experimental,
    t,
    tCommon,
    tCli,
  ])
}
