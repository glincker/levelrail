import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { UniversityGlassIcon } from '@thesvg/ui-react'
import {
  organizationListQueryOptions,
  useOrganizations,
} from '../../queries/organizations'
import { OrganizationTable } from '../../components/OrganizationTable'
import { CreateOrganizationDialog } from '../../components/CreateOrganizationDialog'
import { TableSkeleton } from '../../components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

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
  const { t } = useTranslation('settings')
  const { data: organizations } = useOrganizations()

  return (
    <PageTemplate
      icon={<UniversityGlassIcon size="1.4em" />}
      title={t('organizationsPage.title')}
      description={t('organizationsPage.description')}
      actions={<CreateOrganizationDialog />}
    >
      <OrganizationTable
        organizations={organizations}
        action={<CreateOrganizationDialog />}
      />
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase, matching
// OrganizationTable's own 3-column shape so the skeleton doesn't jump
// when real rows swap in.
function OrganizationsPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="organizations" />
      <TableSkeleton columnCount={3} />
    </div>
  )
}
