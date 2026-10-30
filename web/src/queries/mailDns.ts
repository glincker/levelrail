// Mail DNS helper (internal/api/mail_dns.go): dry-run plan, hash-pinned apply
// and a live check. Mirrors internal/mailrecords wire shapes.

import { ApiError, readErrorMessage } from '../lib/apiError'

export type MailPlanAction = 'add' | 'unchanged' | 'replace' | 'refused'
export type MailCheckStatus = 'pass' | 'warn' | 'fail' | 'not_tested'

export interface MailRecordSet {
  name: string
  type: string
  ttl: number
  values: string[]
}

export interface MailPlanChange {
  action: MailPlanAction
  kind: string
  set: MailRecordSet
  reason?: string
}

export interface MailPlanNote {
  level: string
  code: string
  message: string
}

export interface MailPlan {
  domain: string
  hash: string
  changes: MailPlanChange[]
  notes?: MailPlanNote[]
  refusals?: string[]
  summary: Record<string, number>
  blocked: boolean
}

export interface MailApplyResult {
  applied: number
  plan: MailPlan
}

export interface MailCheckItem {
  id: string
  status: MailCheckStatus
  message: string
  value?: string
}

export interface MailCheckReport {
  domain: string
  items: MailCheckItem[]
}

export interface MailDnsSpec {
  domain: string
  preset?: string
  mail_host?: string
  ipv4?: string
}

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `mail dns request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function post<T>(path: string, body: unknown): Promise<T> {
  return send<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export const planMailDns = (spec: MailDnsSpec) =>
  post<MailPlan>('/api/v1/mail/dns/plan', spec)

export const applyMailDns = (spec: MailDnsSpec, planHash: string) =>
  post<MailApplyResult>('/api/v1/mail/dns/apply', {
    ...spec,
    confirm: true,
    plan_hash: planHash,
  })

export function checkMailDns(spec: MailDnsSpec): Promise<MailCheckReport> {
  const q = new URLSearchParams({ domain: spec.domain })
  if (!spec.preset && spec.mail_host) q.set('mail_host', spec.mail_host)
  return send<MailCheckReport>(`/api/v1/mail/dns/check?${q.toString()}`)
}
