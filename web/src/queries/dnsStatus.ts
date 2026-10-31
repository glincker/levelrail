// Per-record live status: GET /api/v1/domains/dns-status?domain=&app= and
// GET /api/v1/mail/dns/status with the plan spec as query params (internal/dnsstatus).
// Any non-OK answer (older server, mail flag off) yields null so the UI falls
// back to the plain plan diff.

import type { RowStatus } from '../components/dns/dnsSetup'

export interface DnsFoundRecord {
  type: string
  name: string
  value: string
  ttl?: number
  proxied?: boolean
}

export interface DnsRecordStatus {
  id: string
  type: string
  name: string
  value: string
  ttl: number
  priority?: number
  status: RowStatus
  found: DnsFoundRecord[]
  reason?: string
  checked_at: string
}

export interface DnsStatusCounts {
  live: number
  waiting: number
  different: number
  conflict: number
  unknown: number
  total: number
}

export interface DnsStatusReport {
  provider_source: string
  checked_at: string
  counts: DnsStatusCounts
  records: DnsRecordStatus[]
}

/** The plan spec fields the mail status route reads from the query string. */
export interface MailStatusSpec {
  domain: string
  mail_host?: string
  ipv4?: string
  preset?: string
}

export async function fetchDnsStatus(
  kind: 'mail' | 'app',
  domain: string,
  app?: string,
  spec?: MailStatusSpec,
): Promise<DnsStatusReport | null> {
  const q = new URLSearchParams({ domain })
  if (kind === 'app' && app) q.set('app', app)
  if (kind === 'mail' && spec) {
    for (const k of ['mail_host', 'ipv4', 'preset'] as const) {
      const v = spec[k]
      if (v) q.set(k, v)
    }
  }
  const path =
    kind === 'mail' ? '/api/v1/mail/dns/status' : '/api/v1/domains/dns-status'
  try {
    const res = await fetch(`${path}?${q.toString()}`)
    if (!res.ok) return null
    const body = (await res.json()) as DnsStatusReport
    return Array.isArray(body.records) ? body : null
  } catch {
    return null
  }
}

const norm = (s: string) => s.toLowerCase().replace(/\.$/, '')

export function statusFor(
  report: DnsStatusReport | null | undefined,
  name: string,
  type: string,
): DnsRecordStatus | undefined {
  return report?.records.find(
    (r) =>
      norm(r.name) === norm(name) &&
      r.type.toUpperCase() === type.toUpperCase(),
  )
}

/** Header progress: the server's own tally when present, else counted from the rows. */
export function progressOf(
  rows: RowStatus[],
  report?: DnsStatusReport | null,
): { live: number; total: number } {
  if (report?.counts && report.counts.total > 0) {
    return { live: report.counts.live, total: report.counts.total }
  }
  return { live: rows.filter((r) => r === 'live').length, total: rows.length }
}
