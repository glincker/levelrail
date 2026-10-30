import { lazy, Suspense, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LinkIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { NewAppWizard } from '../../components/newapp/NewAppWizard'
import {
  DeployLinkForm,
  DeployLinkNotice,
} from '../../components/newapp/DeployLinkForm'
import { DeployLinkDialog } from '../../components/newapp/DeployLinkDialog'
import { useDeployLinkPrefill } from '../../hooks/useDeployLinkPrefill'
import { stringifySearch } from '../../lib/deployLink'

const DeployButtonBuilder = lazy(
  () => import('../../components/newapp/DeployButtonBuilder'),
)

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
  const [linkOpen, setLinkOpen] = useState(false)
  const { t: tl } = useTranslation('newApp')

  if (prefill.active && prefill.image !== undefined) {
    return (
      <PageTemplate title={t('title')} description={t('description')}>
        <DeployLinkForm prefill={prefill} />
      </PageTemplate>
    )
  }
  return (
    <PageTemplate
      title={t('title')}
      description={t('description')}
      width="wide"
    >
      {prefill.active ? (
        <div className="mx-auto w-full max-w-3xl space-y-3">
          <DeployLinkNotice prefill={prefill} />
          <p className="text-sm text-muted-foreground">
            {t('deployLink.missingImage')}
          </p>
        </div>
      ) : null}
      <NewAppWizard />
      {prefill.active ? null : (
        <>
          <div className="mx-auto w-full max-w-3xl">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setLinkOpen(true)
              }}
            >
              <LinkIcon aria-hidden />
              {tl('deployLinkDialog.action')}
            </Button>
          </div>
          {linkOpen ? (
            <DeployLinkDialog open onOpenChange={setLinkOpen} />
          ) : null}
          <Suspense fallback={null}>
            <DeployButtonBuilder />
          </Suspense>
        </>
      )}
    </PageTemplate>
  )
}
