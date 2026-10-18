import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { NewAppWizard } from '../../components/newapp/NewAppWizard'
import { StarterTemplatesDialog } from '../../components/newapp/StarterTemplatesDialog'
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
      <PageTemplate title={t('title')} description={t('description')}>
        <DeployLinkForm prefill={prefill} />
      </PageTemplate>
    )
  }
  return (
    <PageTemplate title={t('title')} description={t('description')}>
      {prefill.active ? (
        <div className="mx-auto w-full max-w-3xl space-y-3">
          <DeployLinkNotice prefill={prefill} />
          <p className="text-sm text-muted-foreground">
            {t('deployLink.missingImage')}
          </p>
        </div>
      ) : null}
      <div className="mx-auto mb-3 flex w-full max-w-3xl justify-end">
        <StarterTemplatesDialog />
      </div>
      <NewAppWizard />
    </PageTemplate>
  )
}
