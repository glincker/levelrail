import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageHeader } from '@/components/shell/PageHeader'
import { InboxView } from '../components/inbox/InboxView'

export const Route = createFileRoute('/inbox')({
  component: InboxPage,
})

function InboxPage() {
  const { t } = useTranslation('inbox')
  return (
    <div className="space-y-6">
      <PageHeader title={t('page.title')} description={t('page.description')} />
      <InboxView />
    </div>
  )
}
