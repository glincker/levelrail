import { CertificatesCard } from '../settings/CertificatesCard'
import { EnableHttpsCard } from '../EnableHttpsCard'
import { PLATFORM_ANCHORS } from '../../lib/platformSettingsIndex'
import { PlatformCardSection } from './PlatformCardSection'

export function PlatformHttpsPage() {
  return (
    <>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.enableHttps}>
        <EnableHttpsCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.certificates}>
        <CertificatesCard />
      </PlatformCardSection>
    </>
  )
}
