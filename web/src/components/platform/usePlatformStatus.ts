import { useQuery } from '@tanstack/react-query'
import { dashboardUrlQueryOptions } from '../../queries/dashboardUrl'
import { cloudflareDnsSettingsQueryOptions } from '../../queries/cloudflareDns'
import { googleCloudDnsQueryOptions } from '../../queries/googleCloudDns'
import { ingressSettingsQueryOptions } from '../../queries/domains'
import { useIngressConnectivity } from '../../queries/domainWizard'
import { route53DnsSettingsQueryOptions } from '../../queries/route53Dns'

export type DnsProviderName = 'Cloudflare' | 'Route 53' | 'Google Cloud DNS'

// Same queries the cards use, so the overview and the Domains strip never
// disagree with the pages they link to.
export function usePlatformStatus(opts: { connectivity?: boolean } = {}) {
  const ingress = useQuery(ingressSettingsQueryOptions())
  const dashboard = useQuery(dashboardUrlQueryOptions())
  const cloudflare = useQuery(cloudflareDnsSettingsQueryOptions())
  const route53 = useQuery(route53DnsSettingsQueryOptions())
  const google = useQuery(googleCloudDnsQueryOptions())
  const connectivity = useIngressConnectivity(opts.connectivity ?? false)

  const primary = ingress.data?.primary_domain
  const httpsOff = ingress.data
    ? !primary || primary.endsWith('.sslip.io')
    : undefined
  const providers: DnsProviderName[] = []
  if (cloudflare.data?.enabled) providers.push('Cloudflare')
  if (route53.data?.enabled) providers.push('Route 53')
  if (google.data?.enabled) providers.push('Google Cloud DNS')
  const dnsLoaded = Boolean(cloudflare.data && route53.data && google.data)

  return {
    httpsOff,
    domain: primary,
    dashboardUrl: dashboard.data?.dashboard_url,
    dashboardLoaded: Boolean(dashboard.data),
    providers,
    dnsLoaded,
    connectivity: connectivity.data,
  }
}
