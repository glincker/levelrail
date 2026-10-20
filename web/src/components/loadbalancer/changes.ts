import { ALGORITHM_OPTIONS, type LbFormState } from '../../lib/loadBalancer'
import { detectionTimes, weightShares } from './detection'

export interface ChangeChip {
  key: string
  label: string
  before: string
  after: string
  risky?: string
}

interface FieldDef {
  key: string
  label: string
  show: (f: LbFormState) => string
  risky?: (b: LbFormState, a: LbFormState) => string | undefined
}

const on = (v: boolean, detail: string) => (v ? detail : 'off')
const orDash = (s: string) => (s.trim() === '' ? 'unset' : s.trim())
const algoLabel = (f: LbFormState) =>
  ALGORITHM_OPTIONS.find((o) => o.value === f.algorithm)?.label ?? f.algorithm
const STICKY = new Set(['ip_hash', 'cookie', 'uri_hash'])

const FIELDS: FieldDef[] = [
  {
    key: 'algorithm',
    label: 'Algorithm',
    show: algoLabel,
    risky: (b, a) =>
      STICKY.has(b.algorithm) !== STICKY.has(a.algorithm) ||
      (STICKY.has(a.algorithm) && b.algorithm !== a.algorithm)
        ? 'Clients may move to a different replica and lose in-memory sessions.'
        : undefined,
  },
  {
    key: 'cookie',
    label: 'Cookie',
    show: (f) => (f.algorithm === 'cookie' ? orDash(f.cookieName) : 'n/a'),
  },
  {
    key: 'weights',
    label: 'Traffic split',
    show: (f) =>
      f.algorithm === 'weighted'
        ? weightShares(f.weights)
            .map((s) => `${s}%`)
            .join(' / ')
        : 'even',
  },
  {
    key: 'active',
    label: 'Health check',
    show: (f) => on(f.healthEnabled, f.healthPath),
    risky: (b, a) =>
      b.healthEnabled && !a.healthEnabled
        ? 'Failed replicas will keep receiving traffic.'
        : undefined,
  },
  {
    key: 'interval',
    label: 'Check interval',
    show: (f) => on(f.healthEnabled, orDash(f.healthInterval)),
  },
  {
    key: 'timeout',
    label: 'Check timeout',
    show: (f) => on(f.healthEnabled, orDash(f.healthTimeout)),
  },
  {
    key: 'passes',
    label: 'Passes to recover',
    show: (f) => on(f.healthEnabled, orDash(f.healthPasses)),
  },
  {
    key: 'fails',
    label: 'Failures to mark down',
    show: (f) => on(f.healthEnabled, orDash(f.healthFails)),
  },
  {
    key: 'status',
    label: 'Expected status',
    show: (f) => on(f.healthEnabled, orDash(f.healthStatus)),
  },
  {
    key: 'passive',
    label: 'Passive checks',
    show: (f) =>
      on(
        f.passiveEnabled,
        `${orDash(f.maxFails)} fails, skip ${orDash(f.failDuration)}`,
      ),
  },
  {
    key: 'retries',
    label: 'Retries',
    show: (f) =>
      on(
        f.retriesEnabled,
        `${orDash(f.retryCount)} within ${orDash(f.tryDuration)}`,
      ),
  },
  { key: 'slow', label: 'Slow start', show: (f) => orDash(f.slowStart) },
  { key: 'drain', label: 'Drain', show: (f) => orDash(f.drainTimeout) },
  {
    key: 'reqTimeout',
    label: 'Request timeout',
    show: (f) => orDash(f.requestTimeout),
  },
  {
    key: 'rate',
    label: 'Rate limit',
    show: (f) =>
      on(f.rateEnabled, `${orDash(f.rps)}/s, burst ${orDash(f.burst)}`),
    risky: (b, a) =>
      !b.rateEnabled && a.rateEnabled
        ? 'Clients above the limit get 429 responses.'
        : undefined,
  },
  {
    key: 'tls',
    label: 'Upstream TLS',
    show: (f) =>
      on(
        f.tlsEnabled,
        f.tlsInsecure ? 'no verification' : orDash(f.tlsServerName),
      ),
    risky: (b, a) =>
      a.tlsEnabled && a.tlsInsecure && !(b.tlsEnabled && b.tlsInsecure)
        ? 'Certificates of replicas will not be verified.'
        : undefined,
  },
]

export function summarizeChanges(
  before: LbFormState,
  after: LbFormState,
): ChangeChip[] {
  const chips: ChangeChip[] = []
  for (const def of FIELDS) {
    const b = def.show(before)
    const a = def.show(after)
    if (b === a) continue
    chips.push({
      key: def.key,
      label: def.label,
      before: b,
      after: a,
      risky: def.risky?.(before, after),
    })
  }
  return chips
}

export function predictEffect(after: LbFormState, chips: ChangeChip[]): string {
  if (chips.length === 0) return 'No changes.'
  const parts: string[] = []
  if (chips.some((c) => c.key === 'algorithm' || c.key === 'weights')) {
    parts.push(
      after.algorithm === 'weighted'
        ? `New requests split ${weightShares(after.weights).join('/')}.`
        : `New requests follow ${algoLabel(after).toLowerCase()}.`,
    )
  }
  const d = detectionTimes(after)
  if (
    d &&
    chips.some((c) => ['active', 'interval', 'fails', 'passes'].includes(c.key))
  ) {
    parts.push(`${d.text}.`)
  }
  if (chips.some((c) => c.key === 'drain') && after.drainTimeout.trim()) {
    parts.push(`Deploys keep open streams for ${after.drainTimeout.trim()}.`)
  }
  if (parts.length === 0)
    parts.push('Applies to new requests; open connections are not cut.')
  return parts.join(' ')
}
