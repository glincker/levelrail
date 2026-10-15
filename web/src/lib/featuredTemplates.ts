import type { ServiceTemplateListItem } from '../queries/serviceTemplates'
import { TEMPLATE_LOGO_LOADERS } from './templateLogos'

export const FEATURED_COUNT = 8

// Popular, well-tested entries first; the rest of the grid is topped up from
// any other template that has a mark, so the section never renders blank.
const FEATURED_IDS = [
  'ghost',
  'n8n',
  'uptime-kuma',
  'grafana',
  'nextcloud',
  'vaultwarden',
  'ollama',
  'plausible',
]

export function pickFeatured(
  all: ServiceTemplateListItem[],
): ServiceTemplateListItem[] {
  const byId = new Map(all.map((tpl) => [tpl.id, tpl]))
  const picked: ServiceTemplateListItem[] = []
  for (const id of FEATURED_IDS) {
    const tpl = byId.get(id)
    if (tpl) picked.push(tpl)
  }
  for (const tpl of all) {
    if (picked.length >= FEATURED_COUNT) break
    if (tpl.id in TEMPLATE_LOGO_LOADERS && !picked.includes(tpl)) {
      picked.push(tpl)
    }
  }
  return picked.slice(0, FEATURED_COUNT)
}
