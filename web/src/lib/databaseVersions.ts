import type {
  DatabaseVersionInfo,
  DatabaseVersionSource,
} from '../types/databaseVersions'

export type VersionBadge = 'stable' | 'lts' | 'rc' | 'beta' | 'eolSoon' | 'eol'

export type VersionGroupId =
  'recommended' | 'stable' | 'lts' | 'prerelease' | 'eol'

export interface VersionGroup {
  id: VersionGroupId
  versions: DatabaseVersionInfo[]
}

// A supported line ending inside this window is flagged "EOL soon".
export const EOL_SOON_DAYS = 90

const DAY_MS = 24 * 60 * 60 * 1000

function eolTime(v: DatabaseVersionInfo): number | null {
  if (!v.eol) return null
  const t = Date.parse(v.eol)
  return Number.isNaN(t) ? null : t
}

export function isEndOfLife(v: DatabaseVersionInfo, now: Date): boolean {
  if (v.channel === 'eol') return true
  const t = eolTime(v)
  return t !== null && t <= now.getTime()
}

export function isPrerelease(v: DatabaseVersionInfo): boolean {
  return v.channel === 'rc' || v.channel === 'beta'
}

export function badgeFor(v: DatabaseVersionInfo, now: Date): VersionBadge {
  if (isEndOfLife(v, now)) return 'eol'
  if (v.channel === 'rc') return 'rc'
  if (v.channel === 'beta') return 'beta'
  const t = eolTime(v)
  if (t !== null && t - now.getTime() <= EOL_SOON_DAYS * DAY_MS) {
    return 'eolSoon'
  }
  return v.channel === 'lts' ? 'lts' : 'stable'
}

const GROUP_ORDER: VersionGroupId[] = [
  'recommended',
  'stable',
  'lts',
  'prerelease',
  'eol',
]

// Each version lands in exactly one group. Pre-release stays out unless asked for.
export function groupVersions(
  versions: DatabaseVersionInfo[],
  now: Date,
  showPrerelease: boolean,
): VersionGroup[] {
  const buckets: Record<VersionGroupId, DatabaseVersionInfo[]> = {
    recommended: [],
    stable: [],
    lts: [],
    prerelease: [],
    eol: [],
  }
  for (const v of versions) {
    if (isEndOfLife(v, now)) buckets.eol.push(v)
    else if (isPrerelease(v)) {
      if (showPrerelease) buckets.prerelease.push(v)
    } else if (v.recommended) buckets.recommended.push(v)
    else if (v.channel === 'lts') buckets.lts.push(v)
    else buckets.stable.push(v)
  }
  return GROUP_ORDER.filter((id) => buckets[id].length > 0).map((id) => ({
    id,
    versions: buckets[id],
  }))
}

// What the keyboard can reach: everything except end-of-life entries.
export function selectableVersions(groups: VersionGroup[]): string[] {
  return groups
    .filter((g) => g.id !== 'eol')
    .flatMap((g) => g.versions.map((v) => v.version))
}

export function hasPrerelease(versions: DatabaseVersionInfo[]): boolean {
  return versions.some(isPrerelease)
}

export interface SourceLink {
  name: string
  href: string
}

function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

function isHttps(url: string): boolean {
  return url.startsWith('https://')
}

export function sourceLink(source: DatabaseVersionSource): SourceLink | null {
  const url = [source.eol_url, source.release_notes_url].find(
    (u): u is string => u !== undefined && isHttps(u),
  )
  return url ? { name: hostOf(url), href: url } : null
}

export interface RelativeAge {
  value: number
  unit: 'minute' | 'hour' | 'day'
}

export function relativeAge(
  iso: string | undefined,
  now: Date,
): RelativeAge | null {
  if (!iso) return null
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return null
  const minutes = Math.floor(Math.max(0, now.getTime() - t) / 60_000)
  if (minutes < 60) return { value: -minutes, unit: 'minute' }
  const hours = Math.floor(minutes / 60)
  if (hours < 48) return { value: -hours, unit: 'hour' }
  return { value: -Math.floor(hours / 24), unit: 'day' }
}
