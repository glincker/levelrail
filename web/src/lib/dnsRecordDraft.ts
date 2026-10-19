import type { DnsRecord } from '../queries/dnsRecords'

// A record's identity for edit/delete purposes: libdns's own exact-match
// contract (name + type + value together) is the only thing this view
// can key on, since DNS records have no provider-wide ID.
export function recordKey(r: DnsRecord): string {
  return `${r.type}:${r.name}:${r.value}`
}

export type DraftRecord = {
  name: string
  type: string
  value: string
  ttl: string
}

export const EMPTY_DRAFT: DraftRecord = {
  name: '',
  type: 'A',
  value: '',
  ttl: '300',
}

export function draftFromRecord(r: DnsRecord): DraftRecord {
  return {
    name: r.name,
    type: r.type,
    value: r.value,
    ttl: String(r.ttl_seconds),
  }
}

export function toRecord(d: DraftRecord): DnsRecord {
  return {
    name: d.name.trim(),
    type: d.type,
    value: d.value.trim(),
    ttl_seconds: Math.max(0, Number.parseInt(d.ttl, 10) || 0),
  }
}
