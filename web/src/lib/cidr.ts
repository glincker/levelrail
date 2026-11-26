export type SourceKind = 'ipv4' | 'cidr4' | 'ipv6' | 'cidr6'

export interface SourceCheck {
  valid: boolean
  kind?: SourceKind
  // True for RFC1918 ranges: reachable inside a private network only.
  rfc1918: boolean
}

const OCTET = /^(0|[1-9]\d{0,2})$/

function parseV4(s: string): number[] | null {
  const parts = s.split('.')
  if (parts.length !== 4) return null
  const out: number[] = []
  for (const p of parts) {
    if (!OCTET.test(p)) return null
    const n = Number(p)
    if (n > 255) return null
    out.push(n)
  }
  return out
}

function validV6(s: string): boolean {
  if (!/^[0-9a-fA-F:]+$/.test(s) || !s.includes(':')) return false
  const doubles = s.split('::').length - 1
  if (doubles > 1 || s.includes(':::')) return false
  const groups = s.split(':').filter((g) => g !== '')
  if (groups.some((g) => g.length > 4)) return false
  return doubles === 1 ? groups.length < 8 : s.split(':').length === 8
}

function isRfc1918(o: number[]): boolean {
  const [a = -1, b = -1] = o
  return (
    a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)
  )
}

/** checkSource validates one allowed source: an IPv4/IPv6 address or a CIDR block. */
export function checkSource(raw: string): SourceCheck {
  const s = raw.trim()
  const bad: SourceCheck = { valid: false, rfc1918: false }
  if (s === '') return bad
  const [addr = '', prefix, ...rest] = s.split('/')
  if (rest.length > 0) return bad
  if (prefix !== undefined && !/^\d{1,3}$/.test(prefix)) return bad
  const v4 = parseV4(addr)
  if (v4) {
    if (prefix !== undefined && Number(prefix) > 32) return bad
    return {
      valid: true,
      kind: prefix === undefined ? 'ipv4' : 'cidr4',
      rfc1918: isRfc1918(v4),
    }
  }
  if (validV6(addr)) {
    if (prefix !== undefined && Number(prefix) > 128) return bad
    return {
      valid: true,
      kind: prefix === undefined ? 'ipv6' : 'cidr6',
      rfc1918: false,
    }
  }
  return bad
}
