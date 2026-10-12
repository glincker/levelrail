import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { EnvelopeIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import type { EmailSettings } from '../../queries/emailSettings'

const BACKEND_LABEL: Record<Exclude<EmailSettings['backend'], ''>, string> = {
  smtp: 'SMTP',
  ses: 'Amazon SES',
  resend: 'Resend',
}

export function EmailPageShell({
  backend,
  children,
}: {
  backend: EmailSettings['backend']
  children: ReactNode
}) {
  const { t } = useTranslation('settings')

  return (
    <PageTemplate
      icon={<EnvelopeIcon />}
      eyebrow={t('emailPage.eyebrow')}
      title={t('emailPage.title')}
      description={t('emailPage.description')}
      helpPath="/email-notifications"
      helpLabel={t('emailPage.help')}
      status={
        backend === '' ? (
          <Badge variant="muted">{t('emailPage.status.none')}</Badge>
        ) : (
          <Badge variant="success">{BACKEND_LABEL[backend]}</Badge>
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
