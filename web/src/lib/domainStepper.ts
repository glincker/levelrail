import type { DnsProvider } from '../queries/domainCheck'

export type HintProvider = 'cloudflare' | 'route53' | 'namecheap' | 'other'

export const HINT_PROVIDERS: HintProvider[] = [
  'cloudflare',
  'route53',
  'namecheap',
  'other',
]

// Only the server's own connected provider is evidence; a registrar like
// Namecheap cannot be detected from a hostname, so the operator picks it.
export function defaultHintProvider(provider?: DnsProvider): HintProvider {
  if (provider === 'cloudflare' || provider === 'route53') return provider
  return 'other'
}

export function isWildcard(domain: string): boolean {
  return domain.trim().startsWith('*.')
}

// Host label a registrar form expects: Namecheap wants "@" for the apex,
// others take the full name, so only the apex case needs a hint.
export function isApexName(domain: string): boolean {
  return domain.replace(/^\*\./, '').split('.').length === 2
}

export const STEP_IDS = [
  'domain',
  'records',
  'check',
  'port',
  'certificate',
] as const
export type StepId = (typeof STEP_IDS)[number]
