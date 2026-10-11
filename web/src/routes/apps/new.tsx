import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageHeader } from '@/components/shell/PageHeader'
import { NewAppWizard } from '../../components/newapp/NewAppWizard'

export const Route = createFileRoute('/apps/new')({
  component: NewAppPage,
})

function NewAppPage() {
  const { t } = useTranslation('newApp')
  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} />
      <NewAppWizard />
    </div>
  )
}
