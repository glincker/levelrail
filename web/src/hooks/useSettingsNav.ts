import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  visibleSettingsSections,
  type PaletteSettingsItemId,
  type SettingsAreaId,
  type SettingsItemId,
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

const PALETTE_ITEM_IDS: ReadonlySet<string> = new Set<PaletteSettingsItemId>([
  'email-log',
  'network-shares',
])

function itemLabels(
  id: SettingsItemId,
  t: TFunction<'settings'>,
  tExtra: TFunction<'paletteSettings'>,
): { title: string; description: string } {
  if (isPaletteItem(id)) {
    return {
      title: tExtra(`items.${id}.title`),
      description: tExtra(`items.${id}.description`),
    }
  }
  return {
    title: t(`nav.items.${id}.title`),
    description: t(`nav.items.${id}.description`),
  }
}

function isPaletteItem(id: SettingsItemId): id is PaletteSettingsItemId {
  return PALETTE_ITEM_IDS.has(id)
}

// Visible settings areas with their labels resolved through i18n.
export function useSettingsNav(): LabelledSettingsSection[] {
  const { t } = useTranslation('settings')
  const { t: tExtra } = useTranslation('paletteSettings')
  const experimental = useExperimentalFeatures()
  return visibleSettingsSections().map((section) => ({
    id: section.id,
    title: t(`nav.areas.${section.id}.title`),
    description: t(`nav.areas.${section.id}.description`),
    items: section.items.map((item) => ({
      ...item,
      locked: isFeatureLocked(item.feature, experimental),
      ...itemLabels(item.id, t, tExtra),
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

// Adds items the unified search service matched (by href) to the local text match.
export function mergeSettingsHits(
  sections: readonly LabelledSettingsSection[],
  local: readonly LabelledSettingsSection[],
  hrefs: ReadonlySet<string>,
): LabelledSettingsSection[] {
  if (hrefs.size === 0) return [...local]
  const keep = new Set<string>()
  for (const section of local) {
    for (const item of section.items) keep.add(item.to)
  }
  for (const href of hrefs) keep.add(href)
  return sections
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => keep.has(item.to)),
    }))
    .filter((section) => section.items.length > 0)
}
