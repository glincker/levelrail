import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  DatabaseGlassIcon,
  FolderGlassIcon,
  PackageGlassIcon,
  Share2GlassIcon,
} from '@thesvg/ui-react'
import {
  projectDetailQueryOptions,
  useProject,
} from '../../../queries/projects'
import { appListQueryOptions } from '../../../queries/apps'
import { databaseListQueryOptions } from '../../../queries/databases'
import { environmentListQueryOptions } from '../../../queries/environments'
import { projectEnvQueryOptions } from '../../../queries/projectEnv'
import { useOrganizationListOptional } from '../../../queries/organizations'
import {
  AppRow,
  RowSkeleton as AppRowSkeleton,
} from '../../../components/AppRow'
import {
  DatabaseRow,
  RowSkeleton as DatabaseRowSkeleton,
} from '../../../components/DatabaseRow'
import { Breadcrumbs } from '../../../components/Breadcrumbs'
import { DeleteProjectDialog } from '../../../components/DeleteProjectDialog'
import { PauseResumeProjectButton } from '../../../components/PauseResumeProjectButton'
import { MoveToOrganizationDialog } from '../../../components/MoveToOrganizationDialog'
import { RestartProjectButton } from '../../../components/RestartProjectButton'
import { CanvasLink } from '../../../components/canvas/CanvasLink'
import { QuotaCard } from '../../../components/teams/QuotaCard'
import { ProjectSpendCard } from '../../../components/spend/ProjectSpendCard'
import { ProjectEnvironmentsPanel } from '../../../components/ProjectEnvironmentsPanel'
import { ProjectEnvEditor } from '../../../components/ProjectEnvEditor'
import { SharedEnvSecretsCard } from '../../../components/SharedEnvSecretsCard'
import { resourceErrorComponent } from '@/components/resourceErrorComponent'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { buttonVariants } from '@/components/ui/button'
import { EmptyState } from '../../../components/ui/empty-state'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { cn } from '@/lib/utils'

// Project detail route: the project's own name plus every app and
// database currently filed under it. Deliberately not a new,
// server-side filtered endpoint (internal/api/projects.go's own package
// doc comment explains why): this route primes the exact same
// appListQueryOptions()/databaseListQueryOptions() the unfiltered
// /apps and /databases pages already prime, and filters client-side by
// project_id here. That keeps this page's data one request away from
// being already-warm (an operator who visited /apps first pays no
// second fetch at all) and keeps the "additive organization, not a
// forced migration" principle honest: /apps and /databases never
// change shape or behavior because this page exists.
//
// Lives at $id/index.tsx rather than $id.tsx directly so that
// $id/environments/$envId.tsx (the environment detail route) attaches
// straight to root instead of being forced into this page's own Outlet:
// routes/projects/$id.tsx is a thin pass-through layout for exactly that
// reason, see its own comment.
export const Route = createFileRoute('/projects/$id/')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(projectDetailQueryOptions(id)),
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(databaseListQueryOptions()),
      queryClient.ensureQueryData(environmentListQueryOptions(id)),
      queryClient.ensureQueryData(projectEnvQueryOptions(id)),
    ]),
  component: ProjectDetailPage,
  pendingComponent: ProjectDetailPending,
  errorComponent: resourceErrorComponent('project'),
})

function ProjectDetailPage() {
  const { id } = Route.useParams()
  const navigate = useNavigate()
  const { data: project } = useProject(id)
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const { data: databases } = useSuspenseQuery(databaseListQueryOptions())
  // Optional convenience only, see useOrganizationListOptional's own doc
  // comment: resolving org_id to a human-readable name is a display
  // nicety, a failed/slow fetch just means the raw id is shown instead.
  const organizationList = useOrganizationListOptional()
  const organizationName = organizationList.data?.find(
    (o) => o.id === project.org_id,
  )?.name

  const projectApps = apps.filter((a) => a.project_id === id)
  const projectDatabases = databases.filter((d) => d.project_id === id)
  const isEmpty = projectApps.length === 0 && projectDatabases.length === 0

  return (
    <div className="space-y-6">
      <PageTemplate
        breadcrumb={<Breadcrumbs projectId={id} />}
        title={project.name}
        actions={
          <>
            {isEmpty ? null : (
              <Link
                to="/projects/$id/topology"
                params={{ id: project.id }}
                className={cn(
                  buttonVariants({ variant: 'outline', size: 'sm' }),
                )}
              >
                <Share2GlassIcon size="1.2em" aria-hidden="true" />
                Topology
              </Link>
            )}
            <CanvasLink projectId={project.id} />
            {isEmpty ? null : (
              <PauseResumeProjectButton id={project.id} name={project.name} />
            )}
            <RestartProjectButton
              id={project.id}
              disabled={projectApps.length === 0}
            />
            <DeleteProjectDialog
              id={project.id}
              name={project.name}
              onDeleted={() => {
                void navigate({ to: '/projects' })
              }}
            />
          </>
        }
      >
        <div className="mt-2 flex items-center gap-2 text-sm text-muted-foreground">
          <span>Organization:</span>
          {project.org_id ? (
            <Link
              to="/organizations/$id"
              params={{ id: project.org_id }}
              className="text-foreground hover:underline"
            >
              {organizationName ?? project.org_id}
            </Link>
          ) : (
            <span className="italic">none</span>
          )}
          <MoveToOrganizationDialog
            projectId={project.id}
            projectName={project.name}
            currentOrgId={project.org_id}
          />
        </div>
      </PageTemplate>

      <ProjectEnvironmentsPanel projectId={project.id} />
      <QuotaCard scope="project" scopeId={project.id} />
      <ProjectSpendCard projectId={project.id} projectName={project.name} />
      <ProjectEnvEditor projectId={project.id} />
      <SharedEnvSecretsCard scope="project" id={project.id} />

      {isEmpty ? (
        <EmptyState
          icon={<FolderGlassIcon className="size-5.5" />}
          title="Nothing filed under this project yet"
          description="Assign a project when creating a new app or database, or move an existing one here from its own Overview page."
        />
      ) : (
        <>
          {projectApps.length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <PackageGlassIcon className="size-4.5" />
                  Apps
                </CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <div className="overflow-hidden rounded-b-lg border-t border-border">
                  {projectApps.map((app) => (
                    <AppRow key={app.name} app={app} />
                  ))}
                </div>
              </CardContent>
            </Card>
          ) : null}

          {projectDatabases.length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <DatabaseGlassIcon className="size-4.5" />
                  Databases
                </CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <div className="overflow-hidden rounded-b-lg border-t border-border">
                  {projectDatabases.map((database) => (
                    <DatabaseRow key={database.name} database={database} />
                  ))}
                </div>
              </CardContent>
            </Card>
          ) : null}
        </>
      )}
    </div>
  )
}

// fetchProject (queries/projects.ts) throws a plain ApiError for a 404,
// mirroring NodeDetailError/DatabaseDetailError.
// Route-level fallback for the loader's pending phase: the project's own
// name isn't known yet, so this shows a generic title placeholder above
// RowSkeleton rows for both the apps and databases cards, mirroring
// routes/organizations/$id.tsx's own pendingComponent shape.
function ProjectDetailPending() {
  return (
    <div className="space-y-6">
      <div className="h-6 w-48 animate-pulse rounded bg-muted" />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <PackageGlassIcon className="size-4.5" />
            Apps
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-hidden rounded-b-lg border-t border-border">
            {Array.from({ length: 3 }, (_, i) => (
              <AppRowSkeleton key={i} />
            ))}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <DatabaseGlassIcon className="size-4.5" />
            Databases
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-hidden rounded-b-lg border-t border-border">
            {Array.from({ length: 2 }, (_, i) => (
              <DatabaseRowSkeleton key={i} />
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
