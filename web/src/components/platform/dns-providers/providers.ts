import type { DnsDelegated } from '@/types/dns'
import type { DnsDetection } from '@/queries/domainDns'

export type ProviderId = 'cloudflare' | 'route53' | 'googleclouddns'

export const PROVIDER_IDS: ProviderId[] = [
  'cloudflare',
  'route53',
  'googleclouddns',
]

// The hash anchor each provider answers to, e.g. #route53.
export const PROVIDER_HASH: Record<ProviderId, string> = {
  cloudflare: 'cloudflare',
  route53: 'route53',
  googleclouddns: 'google-cloud-dns',
}

// Detection reports "googlecloud"; the settings and zones APIs say "googleclouddns".
const DETECTED_TO_ID: Record<string, ProviderId> = {
  cloudflare: 'cloudflare',
  route53: 'route53',
  googlecloud: 'googleclouddns',
  googleclouddns: 'googleclouddns',
}

export function providerFromDetection(
  d: DnsDetection | undefined,
): ProviderId | null {
  return d ? (DETECTED_TO_ID[d.provider] ?? null) : null
}

export function providerFromHash(hash: string): ProviderId | null {
  const h = hash.replace(/^#/, '')
  return PROVIDER_IDS.find((id) => PROVIDER_HASH[id] === h) ?? null
}

export type TileStatus = 'connected' | 'notConnected' | 'attention'

export interface ProviderState {
  id: ProviderId
  status: TileStatus
  label?: string
  error?: string
}

// Bounds come from the build env so a large estate can be tuned without a code change.
export const RECORD_SEARCH_DEBOUNCE_MS = Math.max(
  0,
  Number(import.meta.env.VITE_DNS_RECORD_SEARCH_DEBOUNCE_MS ?? 250),
)
export const DETECT_AUTO_MAX = Math.max(
  0,
  Number(import.meta.env.VITE_DNS_DETECT_AUTO_MAX ?? 20),
)
export const DETECT_CONCURRENCY = Math.max(
  1,
  Number(import.meta.env.VITE_DNS_DETECT_CONCURRENCY ?? 4),
)

export function delegationChipKey(
  d: DnsDelegated | undefined,
): 'delegated' | 'partial' | 'notDelegated' | 'unknown' {
  if (d === 'yes') return 'delegated'
  if (d === 'partial') return 'partial'
  if (d === 'no') return 'notDelegated'
  return 'unknown'
}

export const hostKey = (p: string) =>
  p === 'googleclouddns' ? 'googlecloud' : p
