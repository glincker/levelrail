import { useQuery } from '@tanstack/react-query'
import { cloudflareDnsSettingsQueryOptions } from '@/queries/cloudflareDns'
import { route53DnsSettingsQueryOptions } from '@/queries/route53Dns'
import { useGoogleCloudDnsSettings } from '@/queries/googleCloudDns'
import type { ProviderId, ProviderState } from './providers'

/** One status per provider, derived from the three existing settings queries; never reads a secret. */
export function useProviderStates(
  testErrors: Partial<Record<ProviderId, string>>,
): { states: Record<ProviderId, ProviderState>; loading: boolean } {
  const cf = useQuery(cloudflareDnsSettingsQueryOptions())
  const r53 = useQuery(route53DnsSettingsQueryOptions())
  const gcp = useGoogleCloudDnsSettings()

  const make = (
    id: ProviderId,
    enabled: boolean | undefined,
    hasCredential: boolean,
    label?: string,
  ): ProviderState => {
    const error = testErrors[id]
    if (error) return { id, status: 'attention', error }
    if (enabled && hasCredential) return { id, status: 'connected', label }
    if (enabled && !hasCredential) return { id, status: 'attention' }
    return { id, status: 'notConnected' }
  }

  return {
    loading: cf.isPending || r53.isPending || gcp.isPending,
    states: {
      cloudflare: make('cloudflare', cf.data?.enabled, !!cf.data?.has_token),
      route53: make(
        'route53',
        r53.data?.enabled,
        !!r53.data?.has_access_key_id && !!r53.data?.has_secret_access_key,
        r53.data?.region,
      ),
      googleclouddns: make(
        'googleclouddns',
        gcp.data?.enabled,
        !!gcp.data?.has_key,
        gcp.data?.client_email ?? gcp.data?.project_id,
      ),
    },
  }
}
