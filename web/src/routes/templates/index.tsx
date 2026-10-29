import { useTranslation } from 'react-i18next'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { PackageGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { TemplateMarketplace } from '../../components/TemplateMarketplace'
import { PageBanner } from '../../components/PageBanner'
import { LazyTemplatesDiagram } from '../../components/banners/LazyDiagrams'
import { Button } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { TemplateDeployPrefill } from '../../components/templates/TemplateDeployPrefill'
import { TemplateDeployResult } from '../../components/templates/TemplateDeployResult'
import { useDeployTemplateNow } from '../../hooks/useDeployTemplateNow'

// Standalone, shareable "what's in the catalog" page, separate from the
// in-wizard picker BrowseTemplatesFields owns: this route has no dialog
// to sit inside, so it's the natural landing target for a template
// card's name/logo link and for sharing the catalog itself outside the
// app (no auth-walled dialog to open first). This page gets the full
// browse/discover treatment (category rail, search, virtualized grid)
// via TemplateMarketplace; the in-wizard step keeps the compact
// ServiceTemplateGrid it already had, since a dialog has no room for a
// sidebar. Only the select/deploy callbacks differ per caller.
export const Route = createFileRoute('/templates/')({
  validateSearch: (search: Record<string, unknown>): { deploy?: string } => ({
    deploy:
      typeof search.deploy === 'string' && search.deploy.trim() !== ''
        ? search.deploy.trim()
        : undefined,
  }),
  component: TemplatesPage,
})

function TemplatesPage() {
  const { t } = useTranslation('templates')
  const navigate = useNavigate()
  const { deploy } = Route.useSearch()
  const deployTemplateNow = useDeployTemplateNow(undefined, {
    showResult: true,
  })
  const newFromTemplate = (
    <CreateResourceWizard
      initialSelected="browse-templates"
      trigger={
        <Button size="sm" variant="outline">
          <PlusGlassIcon size="1.2em" />
          {t('page.new')}
        </Button>
      }
    />
  )

  if (deploy) return <TemplateDeployPrefill slug={deploy} />

  return (
    <PageTemplate
      title={t('page.title')}
      description={t('page.description')}
      banner={
        <PageBanner
          icon={<PackageGlassIcon size="1.2em" />}
          titleKey="templates:page.title"
          descriptionKey="templates:banner.description"
          action={newFromTemplate}
          illustration={<LazyTemplatesDiagram size="hero" />}
        />
      }
    >
      <TemplateMarketplace
        onSelect={(id) => {
          void navigate({ to: '/templates/$id', params: { id } })
        }}
        onDeployNow={(template) => {
          deployTemplateNow.deploy(template.id)
        }}
        deployingId={deployTemplateNow.pendingId}
        emptyAction={newFromTemplate}
      />
      <TemplateDeployResult
        result={deployTemplateNow.deployed?.response ?? null}
        templateId={deployTemplateNow.deployed?.templateId}
        onClose={deployTemplateNow.clearDeployed}
      />
    </PageTemplate>
  )
}
