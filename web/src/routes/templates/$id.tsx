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
import { PageHeader } from '@/components/shell/PageHeader'
import { formatBytes } from '../../lib/format'
import { routeErrorMessage } from '../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

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
  pendingComponent: TemplateDetailSkeleton,
  errorComponent: TemplateDetailError,
})

function TemplateDetailRoute() {
  const { id } = Route.useParams()
  const { data: template } = useServiceTemplate(id)
  const deployTemplateNow = useDeployTemplateNow()

  if (!template) {
    return <TemplateDetailSkeleton />
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
          <PageHeader
            title={template.name}
            status={<Badge variant="outline">{template.category}</Badge>}
            description={template.slogan}
          />
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

// Mirrors the real page's back link, logo/title/badges card, and the
// deploy button row.
function TemplateDetailSkeleton() {
  return (
    <div className="mx-auto max-w-2xl space-y-6" aria-hidden="true">
      <Skeleton className="h-4 w-32" />
      <div className="flex items-start gap-4 rounded-lg border border-border bg-card p-4">
        <Skeleton className="size-12 shrink-0 rounded-md" />
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <Skeleton className="h-5 w-32" />
            <Skeleton className="h-5 w-16 rounded-full" />
          </div>
          <Skeleton className="h-4 w-56" />
          <Skeleton className="h-4 w-40" />
        </div>
      </div>
      <div className="flex justify-end">
        <Skeleton className="h-9 w-36 rounded-md" />
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
