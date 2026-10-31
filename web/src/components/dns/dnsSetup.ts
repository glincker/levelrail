import type {
  MailCheckReport,
  MailPlan,
  MailPlanChange,
  MailRecordSet,
} from '@/queries/mailDns'
import type { DnsDetection } from '@/queries/domainDns'
import { statusFor, type DnsStatusReport } from '@/queries/dnsStatus'

export type SetupStep = 'pick' | 'detect' | 'review' | 'verify'
export type PrimaryAction = 'add' | 'connect' | 'copy' | 'check'
export type DetectState = 'connected' | 'notConnected' | 'unknown'
export type RecordGroup = 'receive' | 'trust' | 'optional' | 'records'

// Retry knobs come from the build env so a slow zone can be tuned without a code change.
export const VERIFY_ATTEMPTS = Math.max(
  1,
  Number(import.meta.env.VITE_DNS_VERIFY_ATTEMPTS ?? 4),
)
export const VERIFY_BASE_MS = Math.max(
  0,
  Number(import.meta.env.VITE_DNS_VERIFY_BASE_MS ?? 2000),
)

const LABEL = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

/** Strips scheme, path, port, `www.` and a trailing dot; returns '' when it is not a host name. */
export function normalizeDomain(raw: string): string {
  let d = raw.trim().toLowerCase()
  d = d.replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
  d = d.split(/[/?#]/)[0] ?? ''
  d = d.replace(/:\d+$/, '').replace(/\.$/, '')
  d = d.replace(/^www\./, '')
  const labels = d.split('.')
  if (labels.length < 2 || !labels.every((l) => LABEL.test(l))) return ''
  return d
}

/** mail.example.com or smtp.example.com: offer example.com for addresses instead. */
export function apexSuggestion(
  domain: string,
): { apex: string; host: string } | null {
  const labels = domain.split('.')
  if (labels.length < 3) return null
  if (labels[0] !== 'mail' && labels[0] !== 'smtp') return null
  return { apex: labels.slice(1).join('.'), host: domain }
}

const BRAND_KEYS: Record<string, string> = {
  cloudflare: 'cloudflare',
  route53: 'route53',
  digitalocean: 'digitalocean',
  googlecloud: 'google-cloud',
  azure: 'azure',
  hetzner: 'hetzner',
  namecheap: 'namecheap',
  godaddy: 'godaddy',
}

/** Brand mark key for a detected host; BrandTile falls back to an initial when the mark is missing. */
export function brandKeyFor(provider: string): string {
  return BRAND_KEYS[provider] ?? provider
}

const LABELS: Record<string, string> = {
  cloudflare: 'Cloudflare',
  route53: 'Amazon Route 53',
  digitalocean: 'DigitalOcean',
  googlecloud: 'Google Cloud DNS',
  azure: 'Azure DNS',
  hetzner: 'Hetzner DNS',
  namecheap: 'Namecheap',
  godaddy: 'GoDaddy',
}

export const providerLabel = (provider: string) => LABELS[provider] ?? provider

export function detectState(d: DnsDetection | undefined): DetectState {
  if (!d || !d.provider || d.provider === 'manual') return 'unknown'
  return d.connected ? 'connected' : 'notConnected'
}

/** The one button of the review step, by detection state and pending count. */
export function primaryActionOf(
  state: DetectState,
  pending: number,
  connectable = true,
): PrimaryAction {
  if (state === 'unknown') return 'copy'
  if (state === 'notConnected') return connectable ? 'connect' : 'copy'
  return pending > 0 ? 'add' : 'check'
}

const RECEIVE = new Set(['MX', 'HOST'])
const TRUST = new Set(['SPF', 'DKIM', 'DMARC'])

export function groupOf(kind: 'mail' | 'app', c: MailPlanChange): RecordGroup {
  if (kind === 'app') return 'records'
  if (RECEIVE.has(c.kind)) return 'receive'
  if (TRUST.has(c.kind)) return 'trust'
  return 'optional'
}

export function groupChanges(
  kind: 'mail' | 'app',
  plan: MailPlan,
): { group: RecordGroup; changes: MailPlanChange[] }[] {
  const order: RecordGroup[] =
    kind === 'app' ? ['records'] : ['receive', 'trust', 'optional']
  return order
    .map((group) => ({
      group,
      changes: plan.changes.filter((c) => groupOf(kind, c) === group),
    }))
    .filter((g) => g.changes.length > 0)
}

export const pendingChanges = (plan: MailPlan) =>
  plan.changes.filter((c) => c.action === 'add' || c.action === 'replace')

const KEY = (kind: string) => `lr.dnsSetup.domain.${kind}`

export function rememberedDomain(kind: string): string {
  try {
    return sessionStorage.getItem(KEY(kind)) ?? ''
  } catch {
    return ''
  }
}

export function rememberDomain(kind: string, domain: string): void {
  try {
    sessionStorage.setItem(KEY(kind), domain)
  } catch {
    // storage can be blocked; the choice is a convenience only
  }
}

export type RowStatus =
  'live' | 'waiting' | 'different' | 'conflict' | 'unknown'

// Which live check item grades a planned record kind (mail only).
const KIND_CHECK: Record<string, string> = {
  MX: 'mx',
  SPF: 'spf',
  DKIM: 'dkim',
  DMARC: 'dmarc',
  MTASTS: 'mta_sts',
  TLSRPT: 'tls_rpt',
  AUTOCONFIG: 'autoconfig',
}

/** Row state from the zone diff, or from public-DNS check items when no provider is connected. */
export function rowStatus(
  c: MailPlanChange,
  report?: MailCheckReport,
  status?: DnsStatusReport | null,
): RowStatus {
  const live = statusFor(status, c.set.name, c.set.type)
  if (live) return live.status
  if (c.action === 'refused') return c.before ? 'different' : 'conflict'
  if (report) {
    const id = KIND_CHECK[c.kind]
    const item = id ? report.items.find((i) => i.id === id) : undefined
    if (item?.status === 'pass') return 'live'
    if (item?.status === 'fail') return 'waiting'
    if (item?.status === 'warn') return 'different'
    return 'unknown'
  }
  if (c.action === 'unchanged') return 'live'
  if (c.action === 'replace') return 'different'
  return 'waiting'
}

export function liveCount(rows: RowStatus[]): number {
  return rows.filter((r) => r === 'live').length
}

/** Increasing re-check intervals; stops after the last one. One named list, env-overridable. */
function delaysFromEnv(): number[] {
  const raw = String(import.meta.env.VITE_DNS_RECHECK_DELAYS_MS ?? '')
  const list = raw
    .split(',')
    .map((x) => Number(x.trim()))
    .filter((n) => Number.isFinite(n) && n >= 0)
  return list.length > 0 ? list : [5000, 10000, 20000, 40000, 60000]
}
export const RECHECK_DELAYS_MS: number[] = delaysFromEnv()

const valueOf = (set: MailRecordSet, v: string) =>
  set.type === 'TXT' && !v.startsWith('"') ? `"${v}"` : v

/** Zone-file style block: name ttl IN type value, one line per value. */
export function zoneFileText(plan: MailPlan): string {
  return plan.changes
    .flatMap((c) =>
      c.set.values.map(
        (v) =>
          `${c.set.name}.\t${c.set.ttl}\tIN\t${c.set.type}\t${valueOf(c.set, v)}`,
      ),
    )
    .join('\n')
}

/** Tab separated table with a header, pastes cleanly into a sheet or a wiki. */
export function tableText(plan: MailPlan): string {
  const head = 'Type\tName\tValue\tTTL'
  const rows = plan.changes.flatMap((c) =>
    c.set.values.map((v) => `${c.set.type}\t${c.set.name}\t${v}\t${c.set.ttl}`),
  )
  return [head, ...rows].join('\n')
}

/** MX values carry the priority first ("10 mail.example.com"). */
export function splitPriority(set: MailRecordSet, v: string) {
  if (set.type !== 'MX') return { priority: '', value: v }
  const [p, ...rest] = v.split(' ')
  return /^\d+$/.test(p ?? '')
    ? { priority: p ?? '', value: rest.join(' ') }
    : { priority: '', value: v }
}

/** The plan as JSON records, for scripts and API clients. */
export function jsonText(plan: MailPlan): string {
  return JSON.stringify(
    plan.changes.flatMap((c) =>
      c.set.values.map((v) => ({
        type: c.set.type,
        name: c.set.name,
        value: v,
        ttl: c.set.ttl,
      })),
    ),
    null,
    2,
  )
}
