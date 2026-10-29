import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { ChevronRightGlassIcon, UniversityGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { DeleteOrganizationDialog } from './DeleteOrganizationDialog'
import type { OrganizationResource } from '../types/organization'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Mirrors NotificationChannelTable's own small, unvirtualized table
// shape: an organization list realistically stays as small as the
// notification-channel or backup-target lists it sits alongside in
// Settings, not the 3-50-app scale ProjectRow's own virtualized list
// targets.
export function OrganizationTable({
  organizations,
  action,
}: {
  organizations: OrganizationResource[]
  action?: ReactNode
}) {
  const { t } = useTranslation('settings')
  if (organizations.length === 0) {
    return (
      <EmptyState
        className="gap-2 border-0 bg-transparent py-4"
        icon={<UniversityGlassIcon className="size-5.5" />}
        title="No organizations yet"
        description="Group related projects under an organization. A project with no organization is just as valid."
        action={action}
      />
    )
  }

  const columns: ResponsiveColumn<OrganizationResource>[] = [
    {
      id: 'name',
      primary: true,
      header: 'Name',
      cell: (org) => (
        <span className="font-medium text-foreground">{org.name}</span>
      ),
    },
    {
      id: 'created',
      header: 'Created',
      priority: 2,
      cell: (org) => (
        <span className="text-muted-foreground">
          {formatDate(org.created_at)}
        </span>
      ),
    },
  ]

  return (
    <ResponsiveTable
      rows={organizations}
      columns={columns}
      getRowId={(org) => org.id}
      ariaLabel={t('tables.organizations')}
      renderActions={(org) => (
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Link to="/organizations/$id" params={{ id: org.id }}>
            <Button type="button" variant="outline" size="sm">
              View
              <ChevronRightGlassIcon className="size-4" aria-hidden="true" />
            </Button>
          </Link>
          <DeleteOrganizationDialog id={org.id} name={org.name} />
        </div>
      )}
    />
  )
}
