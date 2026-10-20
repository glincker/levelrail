import type { DomainResolver } from '../queries/domainCheck'

export interface ExpectedAnswers {
  hosts?: readonly string[]
  ipv4?: readonly string[]
  ipv6?: readonly string[]
}

export type ResolverMatch = 'matches' | 'different' | 'noAnswer'

const normalise = (v: string) => v.trim().toLowerCase().replace(/\.$/, '')

/** Whether a resolver's answer lands on one of the expected targets. */
export function resolverMatch(
  resolver: DomainResolver,
  expected: ExpectedAnswers,
): ResolverMatch {
  const answers = resolver.addresses ?? []
  if (resolver.error || answers.length === 0) return 'noAnswer'
  const want = new Set(
    [
      ...(expected.hosts ?? []),
      ...(expected.ipv4 ?? []),
      ...(expected.ipv6 ?? []),
    ].map(normalise),
  )
  return answers.some((a) => want.has(normalise(a))) ? 'matches' : 'different'
}
