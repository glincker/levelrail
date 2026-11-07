import type { DomainCheckPanelData } from '../DomainDnsCheck'
import type { DnsSource } from '../../queries/domainCheck'
import {
  dnsPointsHere,
  readinessSteps,
  type ReadinessInput,
} from './domainReadiness'

export type PageState =
  'live' | 'waitingDns' | 'wrongDns' | 'certFailed' | 'issuing' | 'starting'

export function pageState(input: ReadinessInput): PageState {
  const [dns, cert, site] = readinessSteps(input)
  if (dns?.state === 'done' && cert?.state === 'done' && site?.state === 'done')
    return 'live'
  if (dns?.state === 'blocked') return 'wrongDns'
  if (dns?.state !== 'done') return 'waitingDns'
  if (cert?.state === 'blocked') return 'certFailed'
  if (cert?.state === 'progress') return 'issuing'
  return 'starting'
}

export type ChipState = 'ok' | 'pending' | 'wrong'
export interface ResolverChip {
  id: string
  state: ChipState
}

// Fixed public resolvers the server queries; names are matched by address.
export const PUBLIC_RESOLVERS = ['1.1.1.1', '8.8.8.8', '9.9.9.9'] as const

function expectedAddresses(data: DomainCheckPanelData): string[] {
  return [...(data.expected_ipv4 ?? []), ...(data.expected_ipv6 ?? [])]
}

function sourceState(
  src: DnsSource | undefined,
  expected: string[],
): ChipState {
  const found = src?.addresses ?? []
  if (found.length === 0) return 'pending'
  if (expected.length === 0) return 'ok'
  return found.some((a) => expected.includes(a)) ? 'ok' : 'wrong'
}

export function resolverChips(data: DomainCheckPanelData): ResolverChip[] {
  const expected = expectedAddresses(data)
  const auth = data.authoritative ?? []
  const states = auth.map((s) => sourceState(s, expected))
  const authState: ChipState = states.includes('wrong')
    ? 'wrong'
    : states.length > 0 && states.every((s) => s === 'ok')
      ? 'ok'
      : 'pending'
  return [
    { id: 'authoritative', state: authState },
    ...PUBLIC_RESOLVERS.map((ip) => ({
      id: ip,
      state: sourceState(
        (data.public ?? data.resolvers)?.find((s) => s.name.includes(ip)),
        expected,
      ),
    })),
  ]
}

// The record exists but at the wrong value: the one case worth a precise sentence.
export function wrongValue(
  data: DomainCheckPanelData,
): { found: string; should: string } | undefined {
  if (
    data.status !== 'resolves_elsewhere' &&
    data.verdict !== 'points_elsewhere'
  )
    return undefined
  const found =
    data.authoritative?.flatMap((s) => s.addresses ?? [])[0] ??
    data.resolved_hosts?.[0]
  const should = data.expected_ipv4?.[0] ?? data.expected_host
  return found && should ? { found, should } : undefined
}

export function nameserverNames(data: DomainCheckPanelData): string[] {
  return (data.authoritative ?? []).map((s) => s.name)
}

export function dnsResolved(input: ReadinessInput): boolean {
  return dnsPointsHere(input)
}

export function openHost(domain: string): string {
  return domain.replace(/^\*\./, '')
}
