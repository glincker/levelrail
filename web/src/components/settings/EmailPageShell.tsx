import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { EnvelopeIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { EmailRail } from './SettingsRails'
import type { EmailSettings } from '../../queries/emailSettings'

export function EmailPageShell({
  settings,
  children,
}: {
  settings: EmailSettings
  children: ReactNode
}) {
  const backend = settings.backend
  const { t } = useTranslation('settings')

  return (
    <PageTemplate
      icon={<EnvelopeIcon />}
      eyebrow={t('emailPage.eyebrow')}
      title={t('emailPage.title')}
      description={t('emailPage.description')}
      aside={settings ? <EmailRail settings={settings} /> : undefined}
      asideLabel={t('emailPage.rail.label')}
      helpPath="/email-notifications"
      helpLabel={t('emailPage.help')}
      status={
        backend === '' ? (
          <Badge variant="muted">{t('emailPage.status.none')}</Badge>
        ) : (
          <Badge variant="success">
            {t(`emailDelivery.providers.${backend}`)}
          </Badge>
        )
      }
      banners={
        backend === '' ? (
          <Banner variant="warning" title={t('emailPage.banner.title')}>
            {t('emailPage.banner.body')}
          </Banner>
        ) : null
      }
    >
      {children}
    </PageTemplate>
  )
}
