import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import {
  FolderGlassIcon,
  GlobeGlassIcon,
  KeyboardGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { usePageActions } from '@/lib/pageActions'
import { openCliModal } from '@/lib/cliModalStore'
import { useTheme, type Theme } from '../components/ThemeProvider'
import {
  mergeSettingsEntries,
  navModelEntries,
  routeEntryItem,
  THEME_ACTION,
  type PaletteItem,
} from '../components/commandPaletteData'
import {
  domainFromPath,
  useTrafficPaletteItems,
} from '../components/trafficPaletteItems'
import { useMailPaletteItems } from '../components/mailPaletteItems'
import { useResourcePaletteItems } from '../components/resourcePaletteItems'
import { useAppTabPaletteItems } from '../components/apps/useAppTabPaletteItems'
import { useSettingsPaletteEntries } from '../lib/settingsPaletteEntries'
import { useExperimentalFeatures } from './useExperimental'

const NEXT_THEME: Record<Theme, Theme> = {
  light: 'dark',
  dark: 'oled',
  oled: 'system',
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
  const mailItems = useMailPaletteItems()
  const settingsEntries = useSettingsPaletteEntries()

  return React.useMemo<PaletteItem[]>(() => {
    const go =
      (
        to: string,
        params?: Record<string, string>,
        search?: Record<string, string>,
        hash?: string,
      ) =>
      () =>
        void navigate(
          search ? { to, params, search, hash } : { to, params, hash },
        )
    const merged = mergeSettingsEntries(settingsEntries)
    const navEntries = navModelEntries(merged, (id, fallback) =>
      tCommon(`shell.nav.${id}`, { defaultValue: fallback }),
    )
    const items: PaletteItem[] = [...merged, ...navEntries].map((e) =>
      routeEntryItem(
        e,
        experimental,
        go,
        tCommon('shell.locked.paletteSubtitle'),
      ),
    )
    const createWords = t('create.keywords')
    for (const i of items) {
      if (i.key.startsWith('action-create-') || i.key === 'action-new-app') {
        i.keywords = `${createWords} ${i.keywords ?? ''}`.trim()
      } else if (i.group === 'Navigate') {
        i.keywords = `${t('goTo')} ${i.keywords ?? ''}`.trim()
      }
    }
    items.push(
      {
        key: 'action-create-domain',
        label: t('create.domain'),
        keywords: createWords,
        group: 'Actions',
        icon: <GlobeGlassIcon size="1.4em" />,
        run: go('/domains'),
      },
      {
        key: 'action-create-project',
        label: t('create.project'),
        keywords: createWords,
        group: 'Actions',
        icon: <FolderGlassIcon size="1.4em" />,
        run: go('/projects'),
      },
    )
    for (const a of pageActions) items.push({ ...a, group: 'Actions' })
    items.push({ ...THEME_ACTION, run: () => setTheme(NEXT_THEME[theme]) })
    if (onShowShortcuts) {
      items.push({
        key: 'action-shortcuts',
        label: t('keyboardShortcuts'),
        keywords: t('keyboardShortcutsKeywords'),
        group: 'Actions',
        icon: <KeyboardGlassIcon size="1.4em" />,
        run: onShowShortcuts,
      })
    }
    items.push({
      key: 'action-cli-setup',
      label: tCli('cliModal.openAction'),
      keywords: tCli('cliModal.openKeywords'),
      group: 'Actions',
      icon: <TerminalGlassIcon size="1.4em" />,
      run: openCliModal,
    })
    items.push(...traffic.base, ...resourceItems, ...appTabItems, ...mailItems)
    return items
  }, [
    navigate,
    setTheme,
    theme,
    traffic.base,
    resourceItems,
    appTabItems,
    mailItems,
    onShowShortcuts,
    pageActions,
    settingsEntries,
    experimental,
    t,
    tCommon,
    tCli,
  ])
}
