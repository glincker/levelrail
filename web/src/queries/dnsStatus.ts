// Per-record live status: GET /api/v1/domains/dns-status?domain=&app= and
// GET /api/v1/mail/dns/status?domain=. Typed from the agreed wire shape; a 404
// (route not deployed yet) yields null so the UI falls back to the plan diff.

import type { RowStatus } from '../components/dns/dnsSetup'

export interface DnsRecordStatus {
  name: string
  type: string
  status: RowStatus
  found?: string[]
}

export interface DnsStatusReport {
  domain: string
  records: DnsRecordStatus[]
  counts?: Record<string, number>
  provider_source?: string
  checked_at?: string
}

export async function fetchDnsStatus(
  kind: 'mail' | 'app',
  domain: string,
  app?: string,
): Promise<DnsStatusReport | null> {
  const q = new URLSearchParams({ domain })
  if (app) q.set('app', app)
  const path =
    kind === 'mail' ? '/api/v1/mail/dns/status' : '/api/v1/domains/dns-status'
  const res = await fetch(`${path}?${q.toString()}`)
  if (!res.ok) return null
  const body = (await res.json()) as DnsStatusReport
  return Array.isArray(body.records) ? body : null
}

export function statusFor(
  report: DnsStatusReport | null | undefined,
  name: string,
  type: string,
): DnsRecordStatus | undefined {
  return report?.records.find(
    (r) => r.name === name && r.type.toUpperCase() === type.toUpperCase(),
  )
}
