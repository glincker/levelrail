import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GetStartedHub } from '../components/getStarted/GetStartedHub'

export const Route = createFileRoute('/get-started')({
  component: GetStartedPage,
})

function GetStartedPage() {
  const { t } = useTranslation('getStarted')
  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-5">
      <header className="space-y-1">
        <h1 className="text-lg font-semibold text-foreground">
          {t('page.title')}
        </h1>
        <p className="text-sm text-muted-foreground">{t('page.subtitle')}</p>
      </header>
      <GetStartedHub />
    </div>
  )
}
