import { useSuspenseQuery } from '@tanstack/react-query'
import { certificatesQueryOptions } from '../../queries/certificates'
import { ingressSettingsQueryOptions } from '../../queries/domains'
import { PLATFORM_ANCHORS } from '../../lib/platformSettingsIndex'
import { IngressConnectivityCard } from '../IngressConnectivityCard'
import { IngressEdgeCard } from '../IngressEdgeCard'
import { IngressSettingsCard } from '../IngressSettingsCard'
import { ReverseProxyGuideCard } from '../ReverseProxyGuideCard'
import { PlatformCardSection } from './PlatformCardSection'

export function PlatformIngressPage() {
  const { data: settings } = useSuspenseQuery(ingressSettingsQueryOptions())
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const primaryCert = settings.primary_domain
    ? certificates.find((c) => c.domain === settings.primary_domain)
    : undefined
  return (
    <>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.ingressSettings}>
        <IngressSettingsCard settings={settings} primaryCert={primaryCert} />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.ingressEdge}>
        <IngressEdgeCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.connectivity}>
        <IngressConnectivityCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.reverseProxy}>
        <ReverseProxyGuideCard />
      </PlatformCardSection>
    </>
  )
}
