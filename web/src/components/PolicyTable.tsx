import { PencilSimpleIcon, ShieldCheckIcon, UsersIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { PolicyFormDialog } from './PolicyFormDialog'
import { ManagePolicyAttachmentsDialog } from './ManagePolicyAttachmentsDialog'
import { DeletePolicyDialog } from './DeletePolicyDialog'
import type { PolicyResource } from '../queries/iamPolicies'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function PolicyTable({ policies }: { policies: PolicyResource[] }) {
  if (policies.length === 0) {
    return (
      <EmptyState
        icon={<ShieldCheckIcon className="size-5" />}
        title="No IAM policies yet"
        description="Create one to grant or deny access scoped to a specific app or database."
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Description</TableHead>
            <TableHead>Updated</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {policies.map((policy) => (
            <TableRow key={policy.id}>
              <TableCell className="font-medium text-foreground">
                {policy.name}
              </TableCell>
              <TableCell className="max-w-xs truncate text-muted-foreground">
                {policy.description || (
                  <span className="text-muted-foreground/60">No description</span>
                )}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDate(policy.updated_at)}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex items-center justify-end gap-2">
                  <ManagePolicyAttachmentsDialog
                    policy={policy}
                    trigger={
                      <Button type="button" variant="outline" size="sm">
                        <UsersIcon />
                        Attachments
                      </Button>
                    }
                  />
                  <PolicyFormDialog
                    policy={policy}
                    trigger={
                      <Button type="button" variant="outline" size="sm">
                        <PencilSimpleIcon />
                        Edit
                      </Button>
                    }
                  />
                  <DeletePolicyDialog policy={policy} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
