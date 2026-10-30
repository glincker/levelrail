export interface ParsedVersion {
  major: number
  minor: number
  patch: number
}

const SEMVER = /^v?(\d+)\.(\d+)\.(\d+)/

export function parseVersion(
  v: string | null | undefined,
): ParsedVersion | null {
  const m = SEMVER.exec((v ?? '').trim())
  if (!m) return null
  return { major: Number(m[1]), minor: Number(m[2]), patch: Number(m[3]) }
}

// Only a patch gap inside one minor line is a real release count; across minors the skipped releases are unknown.
export function releasesBehind(
  current: string | null | undefined,
  latest: string | null | undefined,
): number | null {
  const a = parseVersion(current)
  const b = parseVersion(latest)
  if (!a || !b || a.major !== b.major || a.minor !== b.minor) return null
  const gap = b.patch - a.patch
  return gap > 0 ? gap : null
}

export function behindWarnThreshold(): number {
  const n = Number(import.meta.env.VITE_UPDATE_BEHIND_WARN ?? 3)
  return Number.isFinite(n) && n > 0 ? n : 3
}

export function displayVersion(v: string): string {
  return `v${v.replace(/^v/, '').split(/[-+]/)[0] ?? ''}`
}
