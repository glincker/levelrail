import { createFileRoute, Link } from '@tanstack/react-router'
import {
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  PackageIcon,
  RocketLaunchIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  serviceTemplateQueryOptions,
  useServiceTemplate,
} from '../../queries/serviceTemplates'
import { useDeployTemplateNow } from '../../hooks/useDeployTemplateNow'
import { CATEGORY_ICONS } from '../../components/ServiceTemplateGrid'
import { TemplateLogo } from '../../components/TemplateLogo'
import { RamFitBadge } from '../../components/RamFitBadge'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { formatBytes } from '../../lib/format'
import { routeErrorMessage } from '../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageSpinner } from '@/components/ui/page-spinner'

// Standalone, linkable page for a single catalog template, the one place
// besides the in-wizard preview step (BrowseTemplatesFields) a template
// can be viewed: this route exists so a template can be shared or
// bookmarked without opening the "New resource" dialog first. Deploy
// semantics aren't duplicated here either: the one-click path reuses
// useDeployTemplateNow (the same hook the wizard grid and the /templates
// catalog page use), and the configuration path reuses the existing
// wizard itself, pre-filled via CreateResourceWizard's initialTemplateId.
export const Route = createFileRoute('/templates/$id')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    queryClient.ensureQueryData(serviceTemplateQueryOptions(id)),
  component: TemplateDetailRoute,
  pendingComponent: PageSpinner,
  errorComponent: TemplateDetailError,
})

function TemplateDetailRoute() {
  const { id } = Route.useParams()
  const { data: template } = useServiceTemplate(id)
  const deployTemplateNow = useDeployTemplateNow()

  if (!template) {
    return <PageSpinner />
  }

  const CategoryIcon = CATEGORY_ICONS[template.category] ?? PackageIcon
  const deploying = deployTemplateNow.pendingId === template.id

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <Link
        to="/templates"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeftIcon className="size-3.5" aria-hidden="true" />
        Back to templates
      </Link>

      <div className="flex items-start gap-4 rounded-lg border border-border bg-card p-4">
        <TemplateLogo
          id={template.id}
          className="size-12 shrink-0"
          fallback={
            <CategoryIcon className="size-12 shrink-0 text-muted-foreground" />
          }
        />
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-lg font-semibold text-foreground">
              {template.name}
            </h1>
            <Badge variant="outline">{template.category}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">{template.slogan}</p>
          {template.documentation_url ? (
            <a
              href={template.documentation_url}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-1 text-sm text-primary underline underline-offset-4"
            >
              Documentation
              <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
            </a>
          ) : null}
          <div className="flex flex-wrap items-center gap-2">
            {!!template.recommended_memory_bytes && (
              <>
                <Badge variant="muted">
                  ~{formatBytes(template.recommended_memory_bytes)} RAM
                  recommended
                </Badge>
                <RamFitBadge
                  recommendedMemoryBytes={template.recommended_memory_bytes}
                />
              </>
            )}
            {template.requires_gpu && (
              <Badge variant="muted">Needs NVIDIA GPU</Badge>
            )}
          </div>
        </div>
      </div>

      {deployTemplateNow.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {deployTemplateNow.error?.message}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="flex justify-end">
        {template.requires_configuration ? (
          <CreateResourceWizard
            initialSelected="browse-templates"
            initialTemplateId={template.id}
            trigger={
              <Button type="button">
                <RocketLaunchIcon />
                Configure and deploy
              </Button>
            }
          />
        ) : (
          <Button
            type="button"
            disabled={deploying}
            onClick={() => {
              deployTemplateNow.deploy(template.id)
            }}
          >
            <RocketLaunchIcon />
            {deploying ? 'Deploying...' : 'Deploy now'}
          </Button>
        )}
      </div>
    </div>
  )
}

function TemplateDetailError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link to="/templates" className="mt-2 inline-block underline">
          Back to templates
        </Link>
      </AlertDescription>
    </Alert>
  )
}
