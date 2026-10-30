// Pure helpers for the status page admin UI. Limits mirror the control plane
// tuning (APP_STATUS_*) and can be overridden at build time.

import type { StatusGroup } from '../queries/statusPage'

function envLimit(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

export const STATUS_LIMITS = {
  about: envLimit(import.meta.env.VITE_STATUS_ABOUT_MAX, 1000),
  description: envLimit(import.meta.env.VITE_STATUS_DESCRIPTION_MAX, 200),
  groupName: envLimit(import.meta.env.VITE_STATUS_GROUP_NAME_MAX, 60),
  groups: envLimit(import.meta.env.VITE_STATUS_GROUPS_MAX, 30),
  url: envLimit(import.meta.env.VITE_STATUS_LOGO_URL_MAX, 512),
}

const HEX_RE = /^#[0-9a-fA-F]{6}$/

/** Empty means "use the default accent", which the server accepts. */
export function isValidAccent(value: string): boolean {
  const v = value.trim()
  return v === '' || HEX_RE.test(v)
}

export function isValidLogoUrl(value: string): boolean {
  const v = value.trim()
  if (v === '') return true
  if (v.length > STATUS_LIMITS.url) return false
  try {
    const u = new URL(v)
    return (
      u.protocol === 'https:' &&
      u.host !== '' &&
      u.username === '' &&
      u.hash === ''
    )
  } catch {
    return false
  }
}

export function isValidWebsite(value: string): boolean {
  const v = value.trim()
  if (v === '') return true
  if (v.length > STATUS_LIMITS.url) return false
  try {
    const u = new URL(v)
    return (
      (u.protocol === 'https:' || u.protocol === 'http:') &&
      u.host !== '' &&
      u.username === ''
    )
  } catch {
    return false
  }
}

export function sortGroups(groups: StatusGroup[]): StatusGroup[] {
  return [...groups].sort(
    (a, b) => a.position - b.position || a.name.localeCompare(b.name),
  )
}

/** Moves one group a step and renumbers all positions; returns only the groups that changed. */
export function reorderGroups(
  groups: StatusGroup[],
  id: string,
  dir: -1 | 1,
): StatusGroup[] {
  const list = sortGroups(groups)
  const from = list.findIndex((g) => g.id === id)
  const to = from + dir
  if (from < 0 || to < 0 || to >= list.length) return []
  const next = [...list]
  const moved = list[from]
  if (!moved) return []
  next.splice(from, 1)
  next.splice(to, 0, moved)
  const changed: StatusGroup[] = []
  next.forEach((g, i) => {
    const before = list.find((x) => x.id === g.id)
    if (before?.position !== i) changed.push({ ...g, position: i })
  })
  return changed
}

export function badgeMarkdown(
  pageUrl: string,
  badgeUrl: string,
  label: string,
): string {
  return `[![${label}](${badgeUrl})](${pageUrl})`
}

export function componentBadgeUrl(
  badgeUrl: string,
  componentId: string,
): string {
  return `${badgeUrl}?component=${encodeURIComponent(componentId)}`
}

export interface BrandingValues {
  accent: string
  logo_url: string
  about: string
  website: string
  noindex: boolean
  powered_by: boolean
}

export function brandingValid(v: BrandingValues): boolean {
  return (
    isValidAccent(v.accent) &&
    isValidLogoUrl(v.logo_url) &&
    isValidWebsite(v.website) &&
    v.about.trim().length <= STATUS_LIMITS.about
  )
}
