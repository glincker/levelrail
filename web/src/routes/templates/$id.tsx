import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeftIcon, RocketLaunchIcon } from '@phosphor-icons/react/dist/ssr'
import {
  serviceTemplateQueryOptions,
  useServiceTemplate,
} from '../../queries/serviceTemplates'
import { useComposePreview } from '../../queries/composeStack'
import { TemplateEnvNames } from '../../components/templates/TemplateEnvNames'
import { TemplateFacts } from '../../components/templates/TemplateFacts'
import { TemplateServices } from '../../components/templates/TemplateServices'
import { RelatedTemplates } from '../../components/templates/RelatedTemplates'
import { useDeployTemplateNow } from '../../hooks/useDeployTemplateNow'
import { TemplateLogoTile } from '../../components/TemplateLogoTile'
import { RamFitBadge } from '../../components/RamFitBadge'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { TemplateInstallForm } from '../../components/TemplateInstallForm'
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
  const { t } = useTranslation('templates')
  const { id } = Route.useParams()
  const { data: template } = useServiceTemplate(id)
  const deployTemplateNow = useDeployTemplateNow()
  const preview = useComposePreview(template?.compose ?? '', [])

  if (!template) {
    return <TemplateDetailSkeleton />
  }

  const deploying = deployTemplateNow.pendingId === template.id
  const services = preview.data?.valid ? preview.data.services : []

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <Link
        to="/templates"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeftIcon className="size-3.5" aria-hidden="true" />
        {t('detail.back')}
      </Link>

      <header className="flex flex-wrap items-start gap-4 rounded-xl border border-border bg-card p-5">
        <TemplateLogoTile id={template.id} name={template.name} size="lg" />
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-semibold text-foreground">
              {template.name}
            </h1>
            <Badge variant="outline">{template.category}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">{template.slogan}</p>
          <div className="flex flex-wrap items-center gap-2">
            {template.recommended_memory_bytes ? (
              <>
                <Badge variant="muted">
                  {t('detail.ramRecommended', {
                    size: formatBytes(template.recommended_memory_bytes),
                  })}
                </Badge>
                <RamFitBadge
                  recommendedMemoryBytes={template.recommended_memory_bytes}
                />
              </>
            ) : null}
            {template.requires_gpu ? (
              <Badge variant="muted">{t('detail.needsGpu')}</Badge>
            ) : null}
          </div>
        </div>
      </header>

      {deployTemplateNow.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {deployTemplateNow.error?.message}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="space-y-6">
          {preview.isPending ? (
            <div role="status" aria-label={t('detail.deployed.loading')}>
              <Skeleton className="h-24 w-full" />
            </div>
          ) : services.length > 0 ? (
            <>
              <TemplateServices services={services} />
              <TemplateEnvNames services={services} />
            </>
          ) : (
            <p className="rounded-md border border-dashed border-border p-4 text-sm text-muted-foreground">
              {t('detail.deployed.unavailable')}
            </p>
          )}
          {template.manifest ? (
            <TemplateInstallForm manifest={template.manifest} />
          ) : null}
          <RelatedTemplates id={template.id} category={template.category} />
        </div>

        <aside className="space-y-4 lg:sticky lg:top-4 lg:self-start">
          <section className="space-y-3 rounded-lg border border-border bg-card p-4">
            <h2 className="text-sm font-medium">{t('detail.deploy.title')}</h2>
            {template.manifest ? null : template.requires_configuration ? (
              <>
                <p className="text-xs text-muted-foreground">
                  {t('detail.deploy.configureHint')}
                </p>
                <CreateResourceWizard
                  initialSelected="browse-templates"
                  initialTemplateId={template.id}
                  trigger={
                    <Button type="button" className="w-full">
                      <RocketLaunchIcon />
                      {t('detail.deploy.configure')}
                    </Button>
                  }
                />
              </>
            ) : (
              <>
                <Button
                  type="button"
                  className="w-full"
                  disabled={deploying}
                  onClick={() => {
                    deployTemplateNow.deploy(template.id)
                  }}
                >
                  <RocketLaunchIcon />
                  {deploying
                    ? t('detail.deploy.deploying')
                    : t('detail.deploy.now')}
                </Button>
                <p className="text-xs text-muted-foreground">
                  {t('detail.deploy.trial')}
                </p>
              </>
            )}
          </section>
          <TemplateFacts
            services={services}
            documentationUrl={template.documentation_url}
            recommendedMemoryBytes={template.recommended_memory_bytes}
          />
        </aside>
      </div>
    </div>
  )
}

// Mirrors the real page's back link, logo/title/badges card, and the
// deploy button row.
function TemplateDetailSkeleton() {
  return (
    <div className="mx-auto max-w-5xl space-y-6" aria-hidden="true">
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
