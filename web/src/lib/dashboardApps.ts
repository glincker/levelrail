import type { Tone } from '@/components/kit'
import type { AppListEntry } from '../types/appDetail'

export const VARIANT_TONE: Record<AppListEntry['status']['variant'], Tone> = {
  success: 'success',
  destructive: 'danger',
  muted: 'neutral',
}

const TONE_ORDER: Tone[] = ['danger', 'neutral', 'success']

/** sortApps puts broken apps first so the list doubles as a health overview. */
export function sortApps(apps: AppListEntry[]): AppListEntry[] {
  const rank = (a: AppListEntry) =>
    TONE_ORDER.indexOf(VARIANT_TONE[a.status.variant])
  return [...apps].sort(
    (a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name),
  )
}
