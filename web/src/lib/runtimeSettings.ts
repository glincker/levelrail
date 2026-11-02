import type {
  RuntimeSetting,
  RuntimeSettingCategory,
  RuntimeSettingValue,
  RuntimeSettingWrite,
} from '../types/runtimeSettings'

export const CATEGORY_ORDER: RuntimeSettingCategory[] = [
  'security',
  'networking',
  'deploys',
  'observability',
  'experimental',
  'retention',
]

export function groupByCategory(
  settings: RuntimeSetting[],
): { category: RuntimeSettingCategory; items: RuntimeSetting[] }[] {
  return CATEGORY_ORDER.map((category) => ({
    category,
    items: settings.filter((s) => s.category === category),
  })).filter((g) => g.items.length > 0)
}

export function listFromText(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')
}

export function draftOf(value: RuntimeSettingValue): string {
  if (Array.isArray(value)) return value.join('\n')
  return String(value)
}

/** valueLabelKey names the runtimeSettings key for values that are words, not data. */
export function valueLabelKey(
  value: RuntimeSettingValue,
): 'values.true' | 'values.false' | 'values.empty' | null {
  if (value === true) return 'values.true'
  if (value === false) return 'values.false'
  if (value === '' || (Array.isArray(value) && value.length === 0)) {
    return 'values.empty'
  }
  return null
}

export function toWrite(
  setting: RuntimeSetting,
  draft: string,
): RuntimeSettingWrite {
  return setting.kind === 'string_list' ? listFromText(draft) : draft.trim()
}

/** Only turning a protection on is exempt from the weakening confirmation. */
export function needsConfirm(
  setting: RuntimeSetting,
  next: RuntimeSettingWrite,
): boolean {
  if (setting.risk !== 'weakens_security') return false
  return !(setting.kind === 'bool' && next === true)
}

/** settingItemId maps a registry key to its locale item id: the segment after the category prefix, camel-cased. */
export function settingItemId(key: string): string {
  const rest = key.includes('.') ? key.slice(key.indexOf('.') + 1) : key
  return rest
    .split(/[._-]+/)
    .filter((p) => p !== '')
    .map((p, i) => (i === 0 ? p : p.charAt(0).toUpperCase() + p.slice(1)))
    .join('')
}

export interface RuntimeSettingFilter {
  query: string
  changedOnly: boolean
}

/** Pure filter: matches title, description, key and env name; changedOnly keeps non-default sources. */
export function filterSettings(
  settings: RuntimeSetting[],
  filter: RuntimeSettingFilter,
  text: (s: RuntimeSetting) => { title: string; description: string },
): RuntimeSetting[] {
  const needle = filter.query.trim().toLowerCase()
  return settings.filter((s) => {
    if (filter.changedOnly && s.source === 'default') return false
    if (needle === '') return true
    const { title, description } = text(s)
    return [title, description, s.key, s.env].some((v) =>
      v.toLowerCase().includes(needle),
    )
  })
}

export const categoryAnchor = (category: RuntimeSettingCategory) =>
  `runtime-cat-${category}`
