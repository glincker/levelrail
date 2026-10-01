import { useRef } from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { FolderIcon } from '@phosphor-icons/react/dist/ssr'
import {
  organizationDetailQueryOptions,
  useOrganization,
} from '../../queries/organizations'
import { organizationEnvQueryOptions } from '../../queries/organizationEnv'
import { projectListQueryOptions, useProjects } from '../../queries/projects'
import {
  PROJECT_LIST_GRID,
  ProjectRow,
  RowSkeleton,
} from '../../components/ProjectRow'
import { Breadcrumbs } from '../../components/Breadcrumbs'
import { DeleteOrganizationDialog } from '../../components/DeleteOrganizationDialog'
import { OrganizationEnvEditor } from '../../components/OrganizationEnvEditor'
import { SharedEnvSecretsCard } from '../../components/SharedEnvSecretsCard'
import { routeErrorMessage } from '../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { EmptyState } from '../../components/ui/empty-state'
import { PageHeader } from '@/components/shell/PageHeader'

// Organization detail route: the org's own name plus every project
// currently filed under it, the sibling-navigation surface an operator
// needs once they're looking at one org among several, mirroring
// routes/projects/$id.tsx's own "filter the shared list client-side"
// shape for apps/databases (internal/api/organizations.go has no
// server-side filtered endpoint either).
export const Route = createFileRoute('/organizations/$id')({
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(organizationDetailQueryOptions(id)),
      queryClient.ensureQueryData(projectListQueryOptions()),
      queryClient.ensureQueryData(organizationEnvQueryOptions(id)),
    ]),
  component: OrganizationDetailPage,
  pendingComponent: OrganizationDetailPending,
  errorComponent: OrganizationDetailError,
})

function OrganizationDetailPage() {
  const { id } = Route.useParams()
  const navigate = useNavigate()
  const { data: organization } = useOrganization(id)
  const { data: projects } = useProjects()
  const orgProjects = projects.filter((p) => p.org_id === id)
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: orgProjects.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div className="space-y-6">
      <PageHeader
        breadcrumb={<Breadcrumbs organizationId={id} />}
        title={organization.name}
        actions={
          <DeleteOrganizationDialog
            id={organization.id}
            name={organization.name}
            onDeleted={() => {
              void navigate({ to: '/settings/organizations' })
            }}
          />
        }
      />

      <OrganizationEnvEditor organizationId={id} />
      <SharedEnvSecretsCard scope="organization" id={id} />

      {orgProjects.length === 0 ? (
        <EmptyState
          icon={<FolderIcon className="size-5" />}
          title="No projects filed under this organization yet"
          description="File a project into this organization from the project's own detail page."
          action={
            <Link
              to="/projects"
              className="inline-block text-sm text-foreground underline"
            >
              Go to projects
            </Link>
          }
        />
      ) : (
        <div
          ref={parentRef}
          className="h-[70vh] overflow-auto rounded-lg border border-border bg-card"
        >
          <div
            className={`${PROJECT_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
          >
            <span aria-hidden="true" />
            <span>Name</span>
            <span>Created</span>
            <span aria-hidden="true" />
          </div>
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: 'relative',
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const project = orgProjects[virtualRow.index]
              if (!project) {
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
                  <ProjectRow project={project} />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

// Route-level fallback for the loader's pending phase: the organization's
// own name isn't known yet, so this shows a generic title placeholder
// above RowSkeleton rows for the project list, mirroring
// routes/apps/index.tsx's own pendingComponent shape.
function OrganizationDetailPending() {
  return (
    <div className="space-y-6">
      <div className="h-6 w-48 animate-pulse rounded bg-muted" />
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <div
          className={`${PROJECT_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
        >
          <span aria-hidden="true" />
          <span>Name</span>
          <span>Created</span>
          <span aria-hidden="true" />
        </div>
        {Array.from({ length: 6 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}

function OrganizationDetailError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link
          to="/settings/organizations"
          className="mt-2 inline-block underline"
        >
          Back to organizations
        </Link>
      </AlertDescription>
    </Alert>
  )
}
