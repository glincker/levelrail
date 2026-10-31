import { PLATFORM_ANCHORS } from '../../lib/platformSettingsIndex'
import { AppsBaseDomainCard } from '../AppsBaseDomainCard'
import { DashboardUrlCard } from '../DashboardUrlCard'
import { DomainAutomationCard } from '../DomainAutomationCard'
import { FallbackDomainsCard } from '../FallbackDomainsCard'
import { PlatformCardSection } from './PlatformCardSection'

export function PlatformAddressesPage() {
  return (
    <>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.dashboardUrl}>
        <DashboardUrlCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.appsBaseDomain}>
        <AppsBaseDomainCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.fallbackDomains}>
        <FallbackDomainsCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.domainAutomation}>
        <DomainAutomationCard />
      </PlatformCardSection>
    </>
  )
}
