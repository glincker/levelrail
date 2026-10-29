import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Share2GlassIcon } from '@thesvg/ui-react'
import {
  projectTopologyQueryOptions,
  useLiveProjectTopology,
} from '../../../queries/projectTopology'
import {
  projectDetailQueryOptions,
  useProject,
} from '../../../queries/projects'
import { InfraCanvas } from '../../../components/topology/InfraCanvas'
import { Breadcrumbs } from '../../../components/Breadcrumbs'
import { routeErrorMessage } from '../../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'

// Interactive canvas over GET /api/v1/projects/{id}/topology. Same
// standalone-route shape $id/environments/$envId.tsx uses.
export const Route = createFileRoute('/projects/$id/topology')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(projectDetailQueryOptions(id)),
      queryClient.ensureQueryData(projectTopologyQueryOptions(id)),
    ]),
  component: ProjectTopologyPage,
  pendingComponent: ProjectTopologyPending,
  errorComponent: ProjectTopologyError,
})

function ProjectTopologyPage() {
  const { id } = Route.useParams()
  const { t } = useTranslation('infraCanvas')
  const { data: project } = useProject(id)
  const { data: graph } = useLiveProjectTopology(id)
  const empty = graph.nodes.length === 0

  return (
    <PageTemplate
      breadcrumb={<Breadcrumbs projectId={id} />}
      title={t('title', { name: project.name })}
      description={t('description')}
    >
      {empty ? (
        <EmptyState
          icon={<Share2GlassIcon className="size-5.5" />}
          title={t('empty.title')}
          description={t('empty.description')}
          action={
            <Link
              to="/projects/$id"
              params={{ id }}
              className="text-sm font-medium text-foreground underline underline-offset-4"
            >
              {t('empty.action')}
            </Link>
          }
        />
      ) : (
        <InfraCanvas projectId={id} graph={graph} />
      )}
    </PageTemplate>
  )
}

function ProjectTopologyPending() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div>
        <Skeleton className="h-6 w-56" />
        <Skeleton className="mt-1 h-4 w-96" />
      </div>
      <Skeleton className="h-[28rem] w-full rounded-xl" />
    </div>
  )
}

function ProjectTopologyError({ error }: { error: unknown }) {
  const { t } = useTranslation('infraCanvas')
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p className="font-medium">{t('error.title')}</p>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
