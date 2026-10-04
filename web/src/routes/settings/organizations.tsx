import { createFileRoute } from '@tanstack/react-router'
import { BuildingsIcon } from '@phosphor-icons/react/dist/ssr'
import {
  organizationListQueryOptions,
  useOrganizations,
} from '../../queries/organizations'
import { OrganizationTable } from '../../components/OrganizationTable'
import { CreateOrganizationDialog } from '../../components/CreateOrganizationDialog'
import { TableSkeleton } from '../../components/ui/table-skeleton'
import { PageHeader } from '../../components/shell/PageHeader'

// Account-level, mirroring routes/settings/notification-channels.tsx:
// an organization groups projects, assigned from a project's own detail
// page (MoveToOrganizationDialog), the same "connect/create once here,
// attach it elsewhere by id" shape channels and backup targets share.
export const Route = createFileRoute('/settings/organizations')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(organizationListQueryOptions()),
  component: OrganizationsPage,
  pendingComponent: OrganizationsPending,
})

function OrganizationsPage() {
  const { data: organizations } = useOrganizations()

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <BuildingsIcon className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <PageHeader
            title="Organizations"
            description="Group related projects under an organization. File a project into one from the project's own detail page."
            actions={<CreateOrganizationDialog />}
          />
        </div>
      </div>
      <OrganizationTable
        organizations={organizations}
        action={<CreateOrganizationDialog />}
      />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// OrganizationTable's own 3-column shape so the skeleton doesn't jump
// when real rows swap in.
function OrganizationsPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">Organizations</h1>
      <TableSkeleton columnCount={3} />
    </div>
  )
}
