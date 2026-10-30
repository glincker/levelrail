// DNS onboarding wire shapes: detection (internal/api/dns_provider_detect.go),
// mail domain candidates (mail_dns_onboard.go) and the general plan/apply pair.
// GET /domains/dns-plan and POST /domains/dns-apply mirror the mail plan/apply
// shapes; they are typed from the mail routes, not from a committed handler.

import { ApiError, readErrorMessage } from '../lib/apiError'
import type { MailPlan } from './mailDns'

export interface DnsDetection {
  domain: string
  apex: string
  zone: string
  nameservers: string[]
  provider: string
  domain_connect: { supported: boolean; host?: string; path?: string }
  connected: boolean
  connectable: boolean
  zone_id?: string
  dashboard_url?: string
  can_issue_certificates: boolean
  hint: string
}

export interface MailDnsCandidate {
  domain: string
  source: string
  sources: string[]
  provider?: string
}

export interface MailDnsDomains {
  domains: MailDnsCandidate[]
  warnings?: string[]
}

export interface AppliedRecord {
  name: string
  type: string
  action: string
  ids: string[]
}

export interface DnsApplyResult {
  applied: number
  plan: MailPlan
  records?: AppliedRecord[]
}

async function getJson<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `dns request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export const detectDnsProvider = (domain: string) =>
  getJson<DnsDetection>(
    `/api/v1/domains/dns-provider?domain=${encodeURIComponent(domain)}`,
  )

export const fetchMailDnsDomains = () =>
  getJson<MailDnsDomains>('/api/v1/mail/dns/domains')

interface ServerAddressRecords {
  records?: { type: string; name: string; value: string }[]
}

// The general plan route is not deployed everywhere; without it the records
// come from the server-address suggestions and there is nothing to apply.
export async function planDomainDns(
  domain: string,
  app?: string,
): Promise<MailPlan> {
  const q = new URLSearchParams({ domain })
  if (app) q.set('app', app)
  try {
    return await getJson<MailPlan>(`/api/v1/domains/dns-plan?${q.toString()}`)
  } catch (e) {
    if (!(e instanceof ApiError) || e.status !== 404) throw e
  }
  const addr = await getJson<ServerAddressRecords>(
    `/api/v1/system/server-address?domain=${encodeURIComponent(domain)}`,
  )
  const changes = (addr.records ?? []).map((r) => ({
    action: 'add' as const,
    kind: r.type,
    set: { name: r.name, type: r.type, ttl: 300, values: [r.value] },
  }))
  return { domain, hash: '', changes, summary: {}, blocked: false }
}

export async function applyDomainDns(
  domain: string,
  planHash: string,
  app?: string,
): Promise<DnsApplyResult> {
  const res = await fetch('/api/v1/domains/dns-apply', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      domain,
      app,
      confirm: true,
      plan_hash: planHash,
    }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `dns apply failed: ${res.status}`),
    )
  }
  return (await res.json()) as DnsApplyResult
}
