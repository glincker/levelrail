import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MailGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
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
      icon={<MailGlassIcon size="1.2em" />}
      eyebrow={t('emailPage.eyebrow')}
      title={t('emailPage.title')}
      description={t('emailPage.description')}
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
    >
      {children}
    </PageTemplate>
  )
}
