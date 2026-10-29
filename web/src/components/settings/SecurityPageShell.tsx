import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { SecurityRail } from './SettingsRails'
import { usePasskeys } from '../../queries/passkeys'
import { useTwoFactorStatus } from '../../queries/twoFactor'

export function SecurityPageShell({ children }: { children: ReactNode }) {
  const { t } = useTranslation('settings')
  const { data: twoFactor } = useTwoFactorStatus()
  const { data: passkeys } = usePasskeys()
  const noSecondFactor = !twoFactor.enabled && passkeys.length === 0

  return (
    <PageTemplate
      icon={<ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />}
      eyebrow={t('securityPage.eyebrow')}
      title={t('securityPage.title')}
      description={t('securityPage.description')}
      aside={<SecurityRail />}
      asideLabel={t('securityPage.rail.label')}
      helpPath="/security"
      helpLabel={t('securityPage.help')}
      status={
        <Badge variant={noSecondFactor ? 'warning' : 'success'}>
          {noSecondFactor
            ? t('securityPage.status.open')
            : t('securityPage.status.protected')}
        </Badge>
      }
      banners={
        noSecondFactor ? (
          <Banner variant="warning" title={t('securityPage.banner.title')}>
            {t('securityPage.banner.body')}
          </Banner>
        ) : null
      }
    >
      {children}
    </PageTemplate>
  )
}
