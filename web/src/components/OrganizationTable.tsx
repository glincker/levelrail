import { BuildingsIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
}: {
  organizations: OrganizationResource[]
}) {
  if (organizations.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<BuildingsIcon className="size-5" />}
        title="No organizations yet"
        description="Group related projects under an organization. A project with no organization is just as valid."
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Created</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {organizations.map((org) => (
            <TableRow key={org.id}>
              <TableCell className="font-medium text-foreground">
                {org.name}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDate(org.created_at)}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end">
                  <DeleteOrganizationDialog id={org.id} name={org.name} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
