import { useTranslation } from 'react-i18next'
import {
  visibleSettingsSections,
  type SettingsAreaId,
  type SettingsNavItem,
} from '../lib/settingsNav'
import { isFeatureLocked } from '../lib/experimental'
import { useExperimentalFeatures } from './useExperimental'

export interface LabelledSettingsItem extends SettingsNavItem {
  locked: boolean
  title: string
  description: string
}

export interface LabelledSettingsSection {
  id: SettingsAreaId
  title: string
  description: string
  items: LabelledSettingsItem[]
}

// Visible settings areas with their labels resolved through i18n.
export function useSettingsNav(): LabelledSettingsSection[] {
  const { t } = useTranslation('settings')
  const experimental = useExperimentalFeatures()
  return visibleSettingsSections().map((section) => ({
    id: section.id,
    title: t(`nav.areas.${section.id}.title`),
    description: t(`nav.areas.${section.id}.description`),
    items: section.items.map((item) => ({
      ...item,
      locked: isFeatureLocked(item.feature, experimental),
      title: t(`nav.items.${item.id}.title`),
      description: t(`nav.items.${item.id}.description`),
    })),
  }))
}

// Case-insensitive match on title, description and area name; an area name
// match keeps the whole area.
export function filterSettingsNav(
  sections: readonly LabelledSettingsSection[],
  query: string,
): LabelledSettingsSection[] {
  const q = query.trim().toLowerCase()
  if (!q) {
    return [...sections]
  }
  return sections
    .map((section) =>
      section.title.toLowerCase().includes(q)
        ? section
        : {
            ...section,
            items: section.items.filter(
              (item) =>
                item.title.toLowerCase().includes(q) ||
                item.description.toLowerCase().includes(q),
            ),
          },
    )
    .filter((section) => section.items.length > 0)
}
