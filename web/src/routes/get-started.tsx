import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '../components/shell/PageTemplate'
import { GetStartedHub } from '../components/getStarted/GetStartedHub'

export const Route = createFileRoute('/get-started')({
  component: GetStartedPage,
})

function GetStartedPage() {
  const { t } = useTranslation('getStarted')
  return (
    <PageTemplate title={t('page.title')} description={t('page.subtitle')}>
      <GetStartedHub />
    </PageTemplate>
  )
}
