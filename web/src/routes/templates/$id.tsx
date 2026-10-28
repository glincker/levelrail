import { useMemo } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  RocketLaunchIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  serviceTemplateQueryOptions,
  useServiceTemplate,
} from '../../queries/serviceTemplates'
import { useComposePreview } from '../../queries/composeStack'
import { TemplateFacts } from '../../components/templates/TemplateFacts'
import { TemplateRequirementsCard } from '../../components/templates/TemplateRequirementsCard'
import { TemplateStackFile } from '../../components/templates/TemplateStackFile'
import { TemplateVerifiedBadge } from '../../components/templates/TemplateVerifiedBadge'
import { TemplateTrustBadges } from '../../components/templates/TemplateTrustBadges'
import { TemplateServices } from '../../components/templates/TemplateServices'
import { TemplateWhatYouGet } from '../../components/templates/TemplateWhatYouGet'
import { TemplateDeployForm } from '../../components/templates/TemplateDeployForm'
import { RelatedTemplates } from '../../components/templates/RelatedTemplates'
import { useDeployTemplateNow } from '../../hooks/useDeployTemplateNow'
import { TemplateLogoTile } from '../../components/TemplateLogoTile'
import { RamFitBadge } from '../../components/RamFitBadge'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import {
  appSlug,
  computeTemplateSummary,
  primaryVersion,
} from '../../lib/templateSummary'
import { formatBytes } from '../../lib/format'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { resourceErrorComponent } from '@/components/resourceErrorComponent'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/templates/$id')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    queryClient.ensureQueryData(serviceTemplateQueryOptions(id)),
  component: TemplateDetailRoute,
  pendingComponent: TemplateDetailSkeleton,
  errorComponent: resourceErrorComponent('template'),
})

function TemplateDetailRoute() {
  const { t } = useTranslation('templates')
  const { id } = Route.useParams()
  const { data: template } = useServiceTemplate(id)
  const deployTemplateNow = useDeployTemplateNow()
  const preview = useComposePreview(template?.compose ?? '', [])
  const services = useMemo(
    () => (preview.data?.valid ? preview.data.services : []),
    [preview.data],
  )
  const summary = useMemo(
    () =>
      computeTemplateSummary({
        compose: template?.compose ?? '',
        services,
        recommendedMemoryBytes: template?.recommended_memory_bytes,
      }),
    [template, services],
  )

  if (!template) {
    return <TemplateDetailSkeleton />
  }

  const deploying = deployTemplateNow.pendingId === template.id
  const version = primaryVersion(services)
  const images = services.flatMap((s) => (s.image ? [s.image] : []))

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <Link
        to="/templates"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeftIcon className="size-3.5" aria-hidden="true" />
        {t('detail.back')}
      </Link>

      <header className="flex flex-wrap items-start gap-4 rounded-xl border border-border bg-card p-4 sm:p-5">
        <TemplateLogoTile id={template.id} name={template.name} size="lg" />
        <div className="min-w-0 flex-1 basis-56 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-semibold text-foreground">
              {template.name}
            </h1>
            {version ? (
              <Badge variant="muted" className="font-mono">
                {version}
              </Badge>
            ) : null}
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
            {template.verified ? <TemplateVerifiedBadge /> : null}
            <TemplateTrustBadges images={images} />
            {template.requires_gpu ? (
              <Badge variant="muted">{t('detail.needsGpu')}</Badge>
            ) : null}
            {template.documentation_url ? (
              <a
                href={template.documentation_url}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1 text-sm text-primary underline underline-offset-4"
              >
                {t('detail.facts.docsLink')}
                <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
              </a>
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
        <div className="min-w-0 space-y-6">
          <TemplateWhatYouGet
            summary={summary}
            slug={appSlug(template.id, template.name)}
          />
          <TemplateRequirementsCard requirements={template.requirements} />
          {preview.isPending ? (
            <div role="status" aria-label={t('detail.deployed.loading')}>
              <Skeleton className="h-16 w-full" />
            </div>
          ) : services.length > 0 ? (
            <TemplateServices services={services} />
          ) : null}
          {template.manifest ? (
            <TemplateDeployForm manifest={template.manifest} />
          ) : (
            <section className="space-y-4 rounded-xl border border-border bg-card p-4 sm:p-5">
              <h2 className="text-base font-semibold">
                {t('detail.deploy.title')}
              </h2>
              <p className="text-sm text-muted-foreground">
                {template.requires_configuration
                  ? t('detail.deploy.configureHint')
                  : t('detail.deploy.trial')}
              </p>
              {template.requires_configuration ? (
                <CreateResourceWizard
                  initialSelected="browse-templates"
                  initialTemplateId={template.id}
                  trigger={
                    <Button type="button">
                      <RocketLaunchIcon aria-hidden="true" />
                      {t('detail.deploy.configure')}
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
                  <RocketLaunchIcon aria-hidden="true" />
                  {deploying
                    ? t('detail.deploy.deploying')
                    : t('detail.deploy.now')}
                </Button>
              )}
            </section>
          )}
          <TemplateStackFile
            compose={template.compose}
            slug={appSlug(template.id, template.name)}
          />
          <RelatedTemplates id={template.id} category={template.category} />
        </div>

        <aside className="space-y-4 lg:sticky lg:top-4 lg:self-start">
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
      <Skeleton className="h-40 w-full rounded-xl" />
    </div>
  )
}
