import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { GeneralRail } from './SettingsRails'
import { useSystemStatus } from '../../queries/systemStatus'

export function GeneralPageShell({ children }: { children: ReactNode }) {
  const { t } = useTranslation('settings')
  const { data: status } = useSystemStatus()
  const healthy = status.docker_connected && status.secrets_configured

  return (
    <PageTemplate
      icon={<SettingsGlassIcon size="1.2em" />}
      eyebrow={t('generalPage.eyebrow')}
      title={t('generalPage.title')}
      description={t('generalPage.description')}
      aside={<GeneralRail />}
      asideLabel={t('generalPage.rail.label')}
      helpPath="/troubleshooting"
      helpLabel={t('generalPage.help')}
      status={
        <Badge variant={healthy ? 'success' : 'warning'}>
          {healthy
            ? t('generalPage.status.healthy')
            : t('generalPage.status.attention')}
        </Badge>
      }
      banners={
        !status.docker_connected ? (
          <Banner variant="critical" title={t('generalPage.banner.docker')}>
            {status.docker_error ?? t('generalPage.banner.dockerBody')}
          </Banner>
        ) : !status.secrets_configured ? (
          <Banner variant="warning" title={t('generalPage.banner.secrets')}>
            {t('generalPage.banner.secretsBody')}
          </Banner>
        ) : null
      }
    >
      {children}
    </PageTemplate>
  )
}
