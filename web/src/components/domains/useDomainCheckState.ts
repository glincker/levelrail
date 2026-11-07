import { useQuery } from '@tanstack/react-query'
import { findProxyDomain } from '../../lib/proxyIntegration'
import { useDomainCheck } from '../../queries/domainCheck'
import {
  proxyIntegrationQueryOptions,
  useVerifyProxyDomain,
} from '../../queries/proxyIntegration'

// One cached check plus the proxy row, shared by the inline panel and the domain page.
export function useDomainCheckState(appName: string, domain: string) {
  const { data, isFetching, refetch } = useDomainCheck(appName, domain, {
    poll: false,
  })
  const { data: proxy } = useQuery(proxyIntegrationQueryOptions())
  const proxyRow = proxy ? findProxyDomain(proxy, domain) : undefined
  const verify = useVerifyProxyDomain()
  const onRefetch = () => {
    void refetch()
    if (proxyRow) verify.mutate(domain)
  }
  return { data, isFetching, onRefetch, proxyRow }
}
