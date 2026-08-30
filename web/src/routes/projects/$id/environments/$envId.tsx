import { useRef } from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { PackageIcon, StackSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  projectDetailQueryOptions,
  useProject,
} from '../../../../queries/projects'
import {
  environmentListQueryOptions,
  useEnvironments,
} from '../../../../queries/environments'
import { environmentEnvQueryOptions } from '../../../../queries/environmentEnv'
import { appListQueryOptions } from '../../../../queries/apps'
import { Breadcrumbs } from '../../../../components/Breadcrumbs'
import { DeleteEnvironmentDialog } from '../../../../components/DeleteEnvironmentDialog'
import { EnvironmentEnvEditor } from '../../../../components/EnvironmentEnvEditor'
import { AppRow } from '../../../../components/AppRow'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'

// Environment detail route: the one place an operator can see every app
// tagged with a specific staging/production-style label, plus jump to a
// sibling environment in the same project. Environments have no fetch-
// by-id endpoint of their own (internal/api/environments.go), only
// list-by-project, so this loads the project's whole environment list
// (already cheap, environmentListQueryOptions) and finds this one by id
// client-side, the same shape routes/projects/$id.tsx already uses for
// apps/databases filtered out of their own unfiltered lists.
export const Route = createFileRoute('/projects/$id/environments/$envId')({
  loader: ({ context: { queryClient }, params: { id, envId } }) =>
    Promise.all([
      queryClient.ensureQueryData(projectDetailQueryOptions(id)),
      queryClient.ensureQueryData(environmentListQueryOptions(id)),
      queryClient.ensureQueryData(appListQueryOptions()),
      // A stale/mistyped envId 404s here; caught rather than propagated,
      // since EnvironmentEnvEditor only renders once the component's own
      // client-side existence check (this file's own doc comment) has
      // already passed, and that path should get the friendlier
      // "Environment not found" message below, not this route's generic
      // error page.
      queryClient.ensureQueryData(environmentEnvQueryOptions(envId)).catch(() => undefined),
    ]),
  component: EnvironmentDetailPage,
  errorComponent: EnvironmentDetailError,
})

function EnvironmentDetailPage() {
  const { id, envId } = Route.useParams()
  const navigate = useNavigate()
  const { data: project } = useProject(id)
  const { data: environments } = useEnvironments(id)
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const environment = environments.find((e) => e.id === envId)
  const siblingEnvironments = environments.filter((e) => e.id !== envId)
  const envApps = apps.filter((a) => a.environment_id === envId)
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: envApps.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 64,
    overscan: 8,
  })

  if (!environment) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          <p>Environment not found in project &ldquo;{project.name}&rdquo;.</p>
          <Link
            to="/projects/$id"
            params={{ id }}
            className="mt-2 inline-block underline"
          >
            Back to {project.name}
          </Link>
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <Breadcrumbs projectId={id} environmentId={envId} />
        <div className="mt-1 flex flex-wrap items-center justify-between gap-3">
          <h1 className="flex items-center gap-2 text-lg font-semibold text-foreground">
            <StackSimpleIcon className="size-4 text-muted-foreground" />
            {environment.name}
          </h1>
          <DeleteEnvironmentDialog
            id={environment.id}
            name={environment.name}
            projectId={id}
            onDeleted={() => {
              void navigate({ to: '/projects/$id', params: { id } })
            }}
          />
        </div>
      </div>

      <EnvironmentEnvEditor environmentId={envId} />

      {siblingEnvironments.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            Other environments
          </span>
          {siblingEnvironments.map((sibling) => (
            <Link
              key={sibling.id}
              to="/projects/$id/environments/$envId"
              params={{ id, envId: sibling.id }}
            >
              <Badge variant="outline" className="hover:bg-muted">
                {sibling.name}
              </Badge>
            </Link>
          ))}
        </div>
      ) : null}

      {envApps.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-card/50 px-4 py-16 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <PackageIcon className="size-5" aria-hidden="true" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              No apps tagged with &ldquo;{environment.name}&rdquo; yet
            </p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              Tag an app with this environment from the app&apos;s own
              Overview page.
            </p>
          </div>
        </div>
      ) : (
        <div
          ref={parentRef}
          className="h-[70vh] overflow-auto rounded-lg border border-border bg-card"
        >
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: 'relative',
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const app = envApps[virtualRow.index]
              if (!app) {
                return null
              }
              return (
                <div
                  key={virtualRow.key}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <AppRow app={app} />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

function EnvironmentDetailError({ error }: { error: Error }) {
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
