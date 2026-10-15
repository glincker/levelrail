import { useQuery } from '@tanstack/react-query'
import type { DomainEnforcement } from '../lib/domainEnforcement'
import { domainsQueryOptions } from '../queries/domains'

export function useDomainEnforcement(
  domain: string,
): DomainEnforcement | undefined {
  const { data } = useQuery(domainsQueryOptions())
  if (!Array.isArray(data)) return undefined
  return data.find((d) => d.domain === domain)?.enforcement
}
