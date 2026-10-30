import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { PLATFORM_ANCHORS } from '../../lib/platformSettingsIndex'
import { AppsBaseDomainCard } from '../AppsBaseDomainCard'
import { DashboardUrlCard } from '../DashboardUrlCard'
import { DomainAutomationCard } from '../DomainAutomationCard'
import { FallbackDomainsCard } from '../FallbackDomainsCard'
import { PlatformCardSection } from './PlatformCardSection'
import { PlatformPageShell } from './PlatformPageShell'

export function PlatformAddressesPage() {
  const { t } = useTranslation('platformSettings')
  return (
    <PlatformPageShell
      title={t('pages.addresses.title')}
      description={t('pages.addresses.description')}
      icon={<GlobeGlassIcon size="1.4em" />}
    >
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
    </PlatformPageShell>
  )
}
