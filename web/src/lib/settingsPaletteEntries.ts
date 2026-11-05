import { createElement, useMemo } from 'react'
import type { RouteEntry } from '../components/commandPaletteData'
import {
  useSettingsNav,
  type LabelledSettingsSection,
} from '../hooks/useSettingsNav'

export const SETTINGS_PALETTE_GROUP = 'Settings'

// Built from the settings nav so a page added there is searchable with no
// second edit. The area name rides along as subtitle and match text, since
// the palette group key stays "Settings" for ordering and server dedupe.
export function settingsPaletteEntries(
  sections: readonly LabelledSettingsSection[],
): RouteEntry[] {
  return sections.flatMap((section) =>
    section.items.map((item) => ({
      key: `settings-${item.id}`,
      label: item.title,
      group: SETTINGS_PALETTE_GROUP,
      icon: createElement(item.icon, { className: 'size-[1.4em]' }),
      to: item.to,
      feature: item.feature,
      subtitle: section.title,
      keywords: `${section.title} ${item.description}`,
    })),
  )
}

export function useSettingsPaletteEntries(): RouteEntry[] {
  const sections = useSettingsNav()
  return useMemo(() => settingsPaletteEntries(sections), [sections])
}
