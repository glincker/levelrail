// GET/PUT /api/v1/system/server-address (internal/api/server_address.go):
// the public addresses DNS records must point at, with an operator override.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ServerAddressPair {
  ipv4?: string
  ipv6?: string
}

export interface DnsRecordSuggestion {
  type: 'A' | 'AAAA' | 'CNAME'
  name: string
  value: string
  alternative?: boolean
}

export interface ServerAddress extends ServerAddressPair {
  source: 'override' | 'detected' | 'env' | 'none'
  detected: ServerAddressPair
  override: ServerAddressPair
  public_host?: string
  public_host_source?: string
  behind_nat: boolean
  tls_terminated_upstream: boolean
  apps_base_domain?: string
  domain?: string
  records?: DnsRecordSuggestion[]
}

const PATH = '/api/v1/system/server-address'

export const serverAddressKeys = {
  all: ['server-address'] as const,
  forDomain: (domain: string) => ['server-address', domain] as const,
}

async function fetchServerAddress(domain: string): Promise<ServerAddress> {
  const url = domain ? `${PATH}?domain=${encodeURIComponent(domain)}` : PATH
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch server address failed: ${res.status}`),
    )
  }
  return (await res.json()) as ServerAddress
}

export function serverAddressQueryOptions(domain = '') {
  return queryOptions({
    queryKey: domain
      ? serverAddressKeys.forDomain(domain)
      : serverAddressKeys.all,
    queryFn: () => fetchServerAddress(domain),
    staleTime: 60_000,
  })
}

export function useServerAddress(domain = '', enabled = true) {
  return useQuery({ ...serverAddressQueryOptions(domain), enabled })
}

export function useSetServerAddress() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (override: ServerAddressPair) => {
      const res = await fetch(PATH, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ipv4: override.ipv4 ?? '',
          ipv6: override.ipv6 ?? '',
        }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `save server address failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as ServerAddress
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: serverAddressKeys.all }),
  })
}
