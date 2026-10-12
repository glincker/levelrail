import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageHeader } from '@/components/shell/PageHeader'
import { NewAppWizard } from '../../components/newapp/NewAppWizard'
import {
  DeployLinkForm,
  DeployLinkNotice,
} from '../../components/newapp/DeployLinkForm'
import { useDeployLinkPrefill } from '../../hooks/useDeployLinkPrefill'
import { stringifySearch } from '../../lib/deployLink'

type NewAppSearch = Record<string, string>

export const Route = createFileRoute('/apps/new')({
  validateSearch: (search: Record<string, unknown>): NewAppSearch =>
    stringifySearch(search),
  component: NewAppPage,
})

function NewAppPage() {
  const { t } = useTranslation('newApp')
  const search = Route.useSearch()
  const prefill = useDeployLinkPrefill(search)

  if (prefill.active && prefill.image !== undefined) {
    return (
      <div className="space-y-6">
        <PageHeader title={t('title')} description={t('description')} />
        <DeployLinkForm prefill={prefill} />
      </div>
    )
  }
  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} />
      {prefill.active ? (
        <div className="mx-auto w-full max-w-3xl space-y-3">
          <DeployLinkNotice prefill={prefill} />
          <p className="text-sm text-muted-foreground">
            {t('deployLink.missingImage')}
          </p>
        </div>
      ) : null}
      <NewAppWizard />
    </div>
  )
}
