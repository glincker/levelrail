import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { ServiceTemplateGrid } from '../../components/ServiceTemplateGrid'
import { PageHeader } from '../../components/shell/PageHeader'
import { Button } from '@/components/ui/button'
import { useDeployTemplateNow } from '../../hooks/useDeployTemplateNow'

// Standalone, shareable "what's in the catalog" page, separate from the
// in-wizard picker BrowseTemplatesFields owns: this route has no dialog
// to sit inside, so it's the natural landing target for a template
// card's name/logo link and for sharing the catalog itself outside the
// app (no auth-walled dialog to open first). Search/filter/category
// logic and card rendering are not duplicated: both this page and the
// wizard's step render the same ServiceTemplateGrid, only the
// select/deploy callbacks differ per caller.
export const Route = createFileRoute('/templates/')({
  component: TemplatesPage,
})

function TemplatesPage() {
  const navigate = useNavigate()
  const deployTemplateNow = useDeployTemplateNow()

  return (
    <div className="space-y-6">
      <PageHeader
        title="Service templates"
        description="Curated, one-click services from the catalog. Pick one to see what it deploys."
        actions={
          <CreateResourceWizard
            initialSelected="browse-templates"
            trigger={
              <Button size="sm" variant="outline">
                <PlusIcon />
                New from a template
              </Button>
            }
          />
        }
      />
      <ServiceTemplateGrid
        onSelect={(id) => {
          void navigate({ to: '/templates/$id', params: { id } })
        }}
        onDeployNow={(template) => {
          deployTemplateNow.deploy(template.id)
        }}
        deployingId={deployTemplateNow.pendingId}
      />
    </div>
  )
}
