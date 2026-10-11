import type {
  DnsCapabilities,
  DnsRecordSet,
  DnsRecordType,
  DnsRouting,
} from '../types/dns'

export const RECORD_TYPES: DnsRecordType[] = [
  'A',
  'AAAA',
  'CNAME',
  'TXT',
  'MX',
  'CAA',
  'SRV',
  'NS',
]

export const ROUTING_POLICIES: DnsRouting[] = [
  'simple',
  'weighted',
  'failover',
  'multivalue',
]

export const CAA_TAGS = ['issue', 'issuewild', 'iodef'] as const

export type ValuePart =
  | 'address'
  | 'target'
  | 'text'
  | 'priority'
  | 'weight'
  | 'port'
  | 'flags'
  | 'tag'
  | 'caaValue'

/** Ordered parts each value of a type is made of, matching the server's canonical form. */
export const TYPE_PARTS: Record<DnsRecordType, ValuePart[]> = {
  A: ['address'],
  AAAA: ['address'],
  CNAME: ['target'],
  NS: ['target'],
  TXT: ['text'],
  MX: ['priority', 'target'],
  SRV: ['priority', 'weight', 'port', 'target'],
  CAA: ['flags', 'tag', 'caaValue'],
}

export type ValueParts = Partial<Record<ValuePart, string>>

export function composeValue(type: DnsRecordType, p: ValueParts): string {
  const t = (k: ValuePart) => (p[k] ?? '').trim()
  switch (type) {
    case 'MX':
      return `${t('priority')} ${t('target')}`
    case 'SRV':
      return `${t('priority')} ${t('weight')} ${t('port')} ${t('target')}`
    case 'CAA':
      return `${t('flags') || '0'} ${t('tag')} ${JSON.stringify(t('caaValue'))}`
    case 'TXT':
      return p.text ?? ''
    case 'A':
    case 'AAAA':
      return t('address')
    default:
      return t('target')
  }
}

export function parseValue(type: DnsRecordType, value: string): ValueParts {
  const f = value.trim().split(/\s+/)
  switch (type) {
    case 'MX':
      return { priority: f[0] ?? '', target: f[1] ?? '' }
    case 'SRV':
      return { priority: f[0], weight: f[1], port: f[2], target: f[3] }
    case 'CAA': {
      const rest = value.trim().split(/\s+/).slice(2).join(' ')
      let caaValue = rest
      try {
        caaValue = JSON.parse(rest) as string
      } catch {
        // Unquoted values are passed through as typed.
      }
      return { flags: f[0], tag: f[1], caaValue }
    }
    case 'TXT':
      return { text: value }
    case 'A':
    case 'AAAA':
      return { address: value.trim() }
    default:
      return { target: value.trim() }
  }
}

const IPV4 =
  /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
const HOST =
  /^(@|(\*\.)?([a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9])?\.)*[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9])?\.?)$/i
const NAME =
  /^(@|\*|(\*\.)?([a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9])?)(\.[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9])?)*)$/i

function isIPv6(v: string) {
  if (!/^[0-9a-f:]+$/i.test(v) || (v.match(/::/g) ?? []).length > 1) {
    return false
  }
  const groups = v.split(':')
  return groups.length >= 3 && groups.length <= 8
}

function inRange(v: string | undefined, max: number) {
  if (v === undefined || !/^\d+$/.test(v)) return false
  return Number(v) <= max
}

/** Validation problem codes, each mapped to a dns:validation.* string. */
export type DnsValidationCode =
  | 'name'
  | 'noValues'
  | 'ipv4'
  | 'ipv6'
  | 'host'
  | 'priority'
  | 'weight'
  | 'port'
  | 'caaTag'
  | 'caaValue'
  | 'textTooLong'
  | 'cnameSingle'
  | 'apexNs'
  | 'ttl'
  | 'proxiedType'
  | 'setIdentifier'
  | 'routingWeight'
  | 'failoverRole'

export function validateValue(
  type: DnsRecordType,
  value: string,
): DnsValidationCode | null {
  const p = parseValue(type, value)
  switch (type) {
    case 'A':
      return IPV4.test(p.address ?? '') ? null : 'ipv4'
    case 'AAAA':
      return isIPv6(p.address ?? '') ? null : 'ipv6'
    case 'CNAME':
    case 'NS':
      return HOST.test(p.target ?? '') ? null : 'host'
    case 'TXT':
      return (p.text ?? '').length > 4000 ? 'textTooLong' : null
    case 'MX':
      if (!inRange(p.priority, 65535)) return 'priority'
      return HOST.test(p.target ?? '') ? null : 'host'
    case 'SRV':
      if (!inRange(p.priority, 65535)) return 'priority'
      if (!inRange(p.weight, 65535)) return 'weight'
      if (!inRange(p.port, 65535)) return 'port'
      return HOST.test(p.target ?? '') ? null : 'host'
    case 'CAA':
      if (!CAA_TAGS.includes((p.tag ?? '') as (typeof CAA_TAGS)[number])) {
        return 'caaTag'
      }
      return (p.caaValue ?? '') === '' ? 'caaValue' : null
  }
}

/** validateRecord mirrors the server's dnszones.Normalize checks for instant feedback. */
export function validateRecord(
  r: DnsRecordSet,
  caps: DnsCapabilities,
): DnsValidationCode[] {
  const out: DnsValidationCode[] = []
  const type = r.type as DnsRecordType
  const name = r.name.trim() === '' ? '@' : r.name.trim()
  if (!NAME.test(name)) out.push('name')
  if (type === 'NS' && name === '@') out.push('apexNs')
  const values = r.values.filter((v) => type === 'TXT' || v.trim() !== '')
  if (values.length === 0) out.push('noValues')
  if (type === 'CNAME' && values.length > 1) out.push('cnameSingle')
  for (const v of values) {
    const code = validateValue(type, v)
    if (code && !out.includes(code)) out.push(code)
  }
  const autoTTL = caps.proxied && r.ttl === 1
  if (!autoTTL && r.ttl !== 0 && (r.ttl < 30 || r.ttl > 604800)) {
    out.push('ttl')
  }
  if (r.proxied && !['A', 'AAAA', 'CNAME'].includes(type)) {
    out.push('proxiedType')
  }
  const routing = r.routing ?? 'simple'
  if (routing !== 'simple' && !(r.set_identifier ?? '').trim()) {
    out.push('setIdentifier')
  }
  if (
    routing === 'weighted' &&
    (r.weight === undefined || r.weight < 0 || r.weight > 255)
  ) {
    out.push('routingWeight')
  }
  if (
    routing === 'failover' &&
    r.failover !== 'PRIMARY' &&
    r.failover !== 'SECONDARY'
  ) {
    out.push('failoverRole')
  }
  return out
}

export function recordId(
  r: Pick<DnsRecordSet, 'name' | 'type' | 'set_identifier'>,
) {
  return `${r.name}|${r.type}|${r.set_identifier ?? ''}`
}

export function filterRecords(
  records: DnsRecordSet[],
  type: string,
  query: string,
): DnsRecordSet[] {
  const needle = query.trim().toLowerCase()
  return records.filter((r) => {
    if (type !== '' && r.type !== type) return false
    if (needle === '') return true
    return [r.name, r.set_identifier ?? '', ...r.values]
      .join(' ')
      .toLowerCase()
      .includes(needle)
  })
}

/** fqdn renders a relative record name under its zone. */
export function fqdn(name: string, zone: string) {
  return name === '@' || name === '' ? zone : `${name}.${zone}`
}
