import { lazy, Suspense, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LinkGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { NewAppAside } from '../../components/newapp/NewAppAside'
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
  const [slot, setSlot] = useState<HTMLDivElement | null>(null)

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
      aside={
        prefill.active ? undefined : (
          <>
            <NewAppAside setSlot={setSlot} />
            <div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setLinkOpen(true)
                }}
              >
                <LinkGlassIcon size="1.4em" aria-hidden />
                {t('deployLinkDialog.action')}
              </Button>
            </div>
            <details className="group w-full">
              <summary className="inline-flex min-h-8 cursor-pointer list-none items-center text-sm font-medium text-muted-foreground hover:text-foreground">
                {t('deployButton.title')}
              </summary>
              <div className="pt-3">
                <Suspense fallback={null}>
                  <DeployButtonBuilder />
                </Suspense>
              </div>
            </details>
          </>
        )
      }
      asideLabel={t('aside.label')}
    >
      {prefill.active ? (
        <div className="mx-auto w-full max-w-3xl space-y-3">
          <DeployLinkNotice prefill={prefill} />
          <p className="text-sm text-muted-foreground">
            {t('deployLink.missingImage')}
          </p>
        </div>
      ) : null}
      <NewAppWizard connectSlot={slot} />
      {linkOpen && !prefill.active ? (
        <DeployLinkDialog open onOpenChange={setLinkOpen} />
      ) : null}
    </PageTemplate>
  )
}
