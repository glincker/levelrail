import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftIcon,
  ListMagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { buttonVariants } from '@/components/ui/button'
import { EmailDeliveryLog } from '../../components/email/EmailDeliveryLog'

export const Route = createFileRoute('/settings/email-log')({
  component: EmailLogPage,
})

function EmailLogPage() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<ListMagnifyingGlassIcon />}
      eyebrow={t('emailPage.eyebrow')}
      title={t('emailDelivery.log.title')}
      description={t('emailDelivery.log.description')}
      helpPath="/email-notifications"
      helpLabel={t('emailPage.help')}
      actions={
        <Link
          to="/settings/email"
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          <ArrowLeftIcon className="size-3.5" aria-hidden="true" />
          {t('emailDelivery.log.back')}
        </Link>
      }
    >
      <EmailDeliveryLog />
    </PageTemplate>
  )
}
