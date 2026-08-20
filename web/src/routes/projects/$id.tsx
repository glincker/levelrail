import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  ArrowLeftIcon,
  DatabaseIcon,
  PackageIcon,
} from '@phosphor-icons/react/dist/ssr'
import { projectDetailQueryOptions, useProject } from '../../queries/projects'
import { appListQueryOptions } from '../../queries/apps'
import { databaseListQueryOptions } from '../../queries/databases'
import { environmentListQueryOptions } from '../../queries/environments'
import { useOrganizationListOptional } from '../../queries/organizations'
import { AppRow } from '../../components/AppRow'
import { DatabaseRow } from '../../components/DatabaseRow'
import { DeleteProjectDialog } from '../../components/DeleteProjectDialog'
import { MoveToOrganizationDialog } from '../../components/MoveToOrganizationDialog'
import { ProjectEnvironmentsPanel } from '../../components/ProjectEnvironmentsPanel'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

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
export const Route = createFileRoute('/projects/$id')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(projectDetailQueryOptions(id)),
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(databaseListQueryOptions()),
      queryClient.ensureQueryData(environmentListQueryOptions(id)),
    ]),
  component: ProjectDetailPage,
  errorComponent: ProjectDetailError,
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
      <div>
        <Link
          to="/projects"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" />
          Projects
        </Link>
        <div className="mt-1 flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-lg font-semibold text-foreground">
            {project.name}
          </h1>
          <DeleteProjectDialog
            id={project.id}
            name={project.name}
            onDeleted={() => {
              void navigate({ to: '/projects' })
            }}
          />
        </div>
        <div className="mt-2 flex items-center gap-2 text-sm text-muted-foreground">
          <span>Organization:</span>
          {project.org_id ? (
            <span className="text-foreground">
              {organizationName ?? project.org_id}
            </span>
          ) : (
            <span className="italic">none</span>
          )}
          <MoveToOrganizationDialog
            projectId={project.id}
            projectName={project.name}
            currentOrgId={project.org_id}
          />
        </div>
      </div>

      <ProjectEnvironmentsPanel projectId={project.id} />

      {isEmpty ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-card/50 px-4 py-16 text-center">
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              Nothing filed under this project yet
            </p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              Assign a project when creating a new app or database, or move an
              existing one here from its own Overview page.
            </p>
          </div>
        </div>
      ) : (
        <>
          {projectApps.length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <PackageIcon className="size-4" />
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
                  <DatabaseIcon className="size-4" />
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
function ProjectDetailError({ error }: { error: Error }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{error.message}</p>
        <Link to="/projects" className="mt-2 inline-block underline">
          Back to projects
        </Link>
      </AlertDescription>
    </Alert>
  )
}
