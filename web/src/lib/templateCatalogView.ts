import type { ServiceTemplateListItem } from '../queries/serviceTemplates'

export const ALL_CATEGORIES = 'All'
export const STARTER_KITS_CATEGORY = 'Starter Kits'

export type TemplateSort = 'relevance' | 'name' | 'memory'

/** normalizeText lowercases and strips diacritics so "cafe" finds "Café". */
export function normalizeText(value: string): string {
  return value
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase()
    .trim()
}

function relevance(template: ServiceTemplateListItem, query: string): number {
  const name = normalizeText(template.name)
  if (name.startsWith(query)) return 0
  if (name.includes(query)) return 1
  if (normalizeText(template.category).includes(query)) return 2
  if (normalizeText(template.slogan).includes(query)) return 3
  return -1
}

export function filterTemplates(
  templates: readonly ServiceTemplateListItem[],
  query: string,
  category: string,
  verifiedOnly = false,
): ServiceTemplateListItem[] {
  const q = normalizeText(query)
  return templates.filter(
    (template) =>
      (!verifiedOnly || template.verified === true) &&
      (category === ALL_CATEGORIES || template.category === category) &&
      (q === '' || relevance(template, q) >= 0),
  )
}

export function sortTemplates(
  templates: readonly ServiceTemplateListItem[],
  sort: TemplateSort,
  query = '',
): ServiceTemplateListItem[] {
  const out = [...templates]
  const byName = (a: ServiceTemplateListItem, b: ServiceTemplateListItem) =>
    a.name.localeCompare(b.name)
  if (sort === 'name') return out.sort(byName)
  if (sort === 'memory') {
    const mem = (t: ServiceTemplateListItem) =>
      t.recommended_memory_bytes ?? Number.POSITIVE_INFINITY
    return out.sort((a, b) => {
      const diff = mem(a) - mem(b)
      return diff === 0 || Number.isNaN(diff) ? byName(a, b) : diff
    })
  }
  const q = normalizeText(query)
  if (q === '') return out
  const rank = (t: ServiceTemplateListItem) => {
    const r = relevance(t, q)
    return r < 0 ? 4 : r
  }
  return out.sort((a, b) => rank(a) - rank(b))
}

export function countCategories(
  templates: readonly ServiceTemplateListItem[],
): [string, number][] {
  const counts = new Map<string, number>()
  for (const t of templates)
    counts.set(t.category, (counts.get(t.category) ?? 0) + 1)
  return Array.from(counts.entries()).sort(
    (a, b) => b[1] - a[1] || a[0].localeCompare(b[0]),
  )
}

/** memoryTier maps a recommended-memory advisory to 1..3 meter bars. */
export function memoryTier(bytes: number): 1 | 2 | 3 {
  const mib = bytes / (1024 * 1024)
  if (mib <= 256) return 1
  if (mib <= 1024) return 2
  return 3
}
