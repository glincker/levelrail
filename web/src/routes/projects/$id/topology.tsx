import { createFileRoute, Link } from '@tanstack/react-router'
import { ShareNetworkIcon } from '@phosphor-icons/react/dist/ssr'
import {
  projectTopologyQueryOptions,
  useProjectTopology,
} from '../../../queries/projectTopology'
import {
  projectDetailQueryOptions,
  useProject,
} from '../../../queries/projects'
import { ProjectTopologyView } from '../../../components/topology/ProjectTopologyView'
import { Breadcrumbs } from '../../../components/Breadcrumbs'
import { routeErrorMessage } from '../../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// This project's own topology: how its apps, databases, and shared
// volumes actually relate, derived from existing desired state (see
// internal/api/topology_graph.go's buildTopologyGraph for exactly which
// field backs each edge kind). Lives alongside $id/index.tsx rather than
// a tab on it, the same standalone-route shape
// $id/environments/$envId.tsx already uses for a project sub-page.
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
  const { data: project } = useProject(id)
  const { data: graph } = useProjectTopology(id)
  const empty = graph.nodes.length === 0

  return (
    <div className="space-y-4">
      <PageHeader
        breadcrumb={<Breadcrumbs projectId={id} />}
        title={`${project.name}: Topology`}
        description="How this project's apps, databases, and shared volumes relate, derived from their actual configuration."
      />
      {empty ? (
        <EmptyState
          icon={<ShareNetworkIcon className="size-5" />}
          title="Nothing filed under this project yet"
          description="Assign an app or database to this project to see how they relate here."
          action={
            <Link
              to="/projects/$id"
              params={{ id }}
              className="text-sm font-medium text-foreground underline underline-offset-4"
            >
              Back to project
            </Link>
          }
        />
      ) : (
        <ProjectTopologyView graph={graph} />
      )}
    </div>
  )
}

// The connector lines aren't worth faking, so this mirrors just the
// outer chrome: header, then a row of column-shaped boxes, the same
// shape routes/network/index.tsx's own NetworkPageSkeleton establishes.
function ProjectTopologyPending() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div>
        <Skeleton className="h-6 w-56" />
        <Skeleton className="mt-1 h-4 w-96" />
      </div>
      <div className="flex flex-wrap gap-4">
        {Array.from({ length: 2 }, (_, i) => (
          <div
            key={i}
            className="min-w-[14rem] flex-1 space-y-3 rounded-xl border border-dashed border-border bg-muted/20 p-3"
          >
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-12 w-full rounded-lg" />
            <Skeleton className="h-12 w-full rounded-lg" />
          </div>
        ))}
      </div>
    </div>
  )
}

function ProjectTopologyError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
