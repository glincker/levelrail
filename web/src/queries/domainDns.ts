// DNS onboarding wire shapes: detection (internal/api/dns_provider_detect.go),
// mail domain candidates (mail_dns_onboard.go) and the app plan/apply, which
// use the committed go-live plan and run routes (go_live_handlers.go).

import { ApiError, readErrorMessage } from '../lib/apiError'
import type { MailPlan, MailPlanChange } from './mailDns'

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

// One DNS outcome of the committed go-live routes (internal/api/go_live.go).
interface GoLiveDns {
  dns: string
  planned?: string
  message?: string
  record?: { name: string; type: string; value: string; ttl_seconds: number }
}

interface GoLiveResult {
  dns?: GoLiveDns[]
}

// Marker hash: go-live has no pinned plan, it recomputes add-only on apply.
const GO_LIVE_HASH = 'go-live'
const DEFAULT_TTL = 300

async function postJson<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `dns request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function goLivePlan(domain: string, r: GoLiveResult): MailPlan | null {
  const changes = (r.dns ?? []).flatMap((d) => {
    if (!d.record) return []
    const set = {
      name: d.record.name,
      type: d.record.type,
      ttl: d.record.ttl_seconds || DEFAULT_TTL,
      values: [d.record.value],
    }
    const action: MailPlanChange['action'] =
      d.dns === 'conflict'
        ? 'refused'
        : d.dns === 'unchanged'
          ? 'unchanged'
          : 'add'
    return [{ action, kind: d.record.type, set, reason: d.message }]
  })
  if (changes.length === 0) return null
  return {
    domain,
    hash: GO_LIVE_HASH,
    changes,
    summary: {},
    blocked: false,
  }
}

// An app plans through the committed go-live dry run; without an app, or when
// it yields no record, the records come from the server-address suggestions.
export async function planDomainDns(
  domain: string,
  app?: string,
): Promise<MailPlan> {
  if (app) {
    const out = await postJson<{ plans?: GoLiveResult[] }>(
      `/api/v1/apps/${encodeURIComponent(app)}/domains/go-live/plan`,
      { domain },
    )
    const plan = goLivePlan(domain, out.plans?.[0] ?? {})
    if (plan) return plan
  }
  const addr = await getJson<ServerAddressRecords>(
    `/api/v1/system/server-address?domain=${encodeURIComponent(domain)}`,
  )
  const changes = (addr.records ?? []).map((r) => ({
    action: 'add' as const,
    kind: r.type,
    set: { name: r.name, type: r.type, ttl: DEFAULT_TTL, values: [r.value] },
  }))
  return { domain, hash: '', changes, summary: {}, blocked: false }
}

// Add-only: replace stays false, so an existing different record is reported
// as a conflict instead of being overwritten.
export async function applyDomainDns(
  domain: string,
  planHash: string,
  app?: string,
): Promise<DnsApplyResult> {
  if (!app || planHash !== GO_LIVE_HASH) {
    throw new ApiError(400, 'dns apply needs an app and a go-live plan')
  }
  const out = await postJson<GoLiveResult>(
    `/api/v1/apps/${encodeURIComponent(app)}/domains/${encodeURIComponent(domain)}/go-live`,
    { dns: 'auto', replace: false },
  )
  const done = (out.dns ?? []).filter(
    (d) => d.dns === 'created' || d.dns === 'updated',
  )
  return {
    applied: done.length,
    plan: goLivePlan(domain, out) ?? {
      domain,
      hash: '',
      changes: [],
      summary: {},
      blocked: false,
    },
    records: (out.dns ?? []).flatMap((d) =>
      d.record
        ? [
            {
              name: d.record.name,
              type: d.record.type,
              action: d.dns === 'created' ? 'add' : d.dns,
              ids: [],
            },
          ]
        : [],
    ),
  }
}
