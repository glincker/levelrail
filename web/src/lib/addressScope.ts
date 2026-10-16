export type HostClass =
  | 'public'
  | 'private'
  | 'cgnat'
  | 'loopback'
  | 'linkLocal'
  | 'hostname'
  | 'none'

function parseV4(host: string): number[] | null {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host)
  if (!m) return null
  const parts = m.slice(1).map(Number)
  return parts.every((n) => n <= 255) ? parts : null
}

function classifyV6(host: string): HostClass {
  const h = host.toLowerCase()
  if (h === '::1') return 'loopback'
  if (/^f[cd][0-9a-f]{2}:/.test(h)) return 'private'
  if (/^fe[89ab][0-9a-f]:/.test(h)) return 'linkLocal'
  return 'public'
}

/** Classifies a host string by who can route to it; hostnames are not resolved. */
export function classifyHost(raw: string | undefined | null): HostClass {
  const host = (raw ?? '').trim().replace(/^\[|\]$/g, '')
  if (!host) return 'none'
  const v4 = parseV4(host)
  if (v4) {
    const [a, b] = v4 as [number, number, number, number]
    if (a === 127) return 'loopback'
    if (a === 10 || (a === 172 && b >= 16 && b <= 31)) return 'private'
    if (a === 192 && b === 168) return 'private'
    if (a === 100 && b >= 64 && b <= 127) return 'cgnat'
    if (a === 169 && b === 254) return 'linkLocal'
    return 'public'
  }
  if (host.includes(':')) return classifyV6(host)
  return host.toLowerCase() === 'localhost' ? 'loopback' : 'hostname'
}

/** True for addresses that only work on the same network or over a VPN. */
export function isPrivateClass(c: HostClass): boolean {
  return c === 'private' || c === 'cgnat' || c === 'linkLocal'
}
