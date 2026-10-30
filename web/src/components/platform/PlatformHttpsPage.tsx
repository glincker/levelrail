import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { CertificatesCard } from '../settings/CertificatesCard'
import { EnableHttpsCard } from '../EnableHttpsCard'
import { PLATFORM_ANCHORS } from '../../lib/platformSettingsIndex'
import { PlatformCardSection } from './PlatformCardSection'
import { PlatformPageShell } from './PlatformPageShell'

export function PlatformHttpsPage() {
  const { t } = useTranslation('platformSettings')
  return (
    <PlatformPageShell
      title={t('pages.https.title')}
      description={t('pages.https.description')}
      icon={<LockGlassIcon size="1.4em" />}
    >
      <PlatformCardSection anchor={PLATFORM_ANCHORS.enableHttps}>
        <EnableHttpsCard />
      </PlatformCardSection>
      <PlatformCardSection anchor={PLATFORM_ANCHORS.certificates}>
        <CertificatesCard />
      </PlatformCardSection>
    </PlatformPageShell>
  )
}
