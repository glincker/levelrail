import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'
import { GavelIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import {
  deployApprovalListQueryOptions,
  useApproveDeployApproval,
  useDeployApprovals,
  useRejectDeployApproval,
} from '../../queries/deployApprovals'
import type { DeployApprovalStatus } from '../../types/deployApproval'
import { ApiError } from '../../lib/apiError'

// Cross-app operational queue for the two-person approval gate on a
// deploy/promote into a protected environment (internal/api/
// deploy_approvals.go): the same "top-level, not under Settings"
// placement routes/backups/index.tsx's own doc comment establishes for
// this category of view (what needs attention right now, not
// configuration). PendingDeployApprovalBanner.tsx surfaces the same data
// scoped to one app's own detail page; this is the place to see every
// pending request across every app at once, and to act on decided
// history via the status filter.
export const Route = createFileRoute('/approvals/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(deployApprovalListQueryOptions('pending')),
  component: DeployApprovalsPage,
  pendingComponent: DeployApprovalsPending,
})

const STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: 'pending', label: 'Pending' },
  { value: 'all', label: 'All' },
  { value: 'approved', label: 'Approved' },
  { value: 'rejected', label: 'Rejected' },
  { value: 'expired', label: 'Expired' },
]

function statusBadgeVariant(
  status: DeployApprovalStatus,
): 'warning' | 'success' | 'destructive' | 'muted' {
  switch (status) {
    case 'pending':
      return 'warning'
    case 'approved':
      return 'success'
    case 'rejected':
      return 'destructive'
    default:
      return 'muted'
  }
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function DeployApprovalsPage() {
  const [status, setStatus] = useState('pending')
  const { data: approvals } = useDeployApprovals(status)
  const approve = useApproveDeployApproval()
  const reject = useRejectDeployApproval()
  const [actingID, setActingID] = useState<string | null>(null)

  function handleApprove(id: string, image: string, serviceName: string) {
    setActingID(id)
    approve.mutate(id, {
      onSuccess: () => {
        toast.add({
          title: `Approved: ${serviceName} now targets ${image}.`,
          type: 'success',
        })
      },
      onError: (err) => {
        toast.add({
          title: 'Could not approve.',
          description: err instanceof ApiError ? err.message : String(err),
          type: 'error',
        })
      },
      onSettled: () => setActingID(null),
    })
  }

  function handleReject(id: string) {
    setActingID(id)
    reject.mutate(
      { id },
      {
        onSuccess: () => {
          toast.add({ title: 'Deploy approval rejected.', type: 'success' })
        },
        onError: (err) => {
          toast.add({
            title: 'Could not reject.',
            description: err instanceof ApiError ? err.message : String(err),
            type: 'error',
          })
        },
        onSettled: () => setActingID(null),
      },
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GavelIcon className="size-4" aria-hidden="true" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Deploy approvals
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              The two-person gate a deploy or promote into a protected
              environment goes through. The same user or token that requested
              one cannot also approve or reject it.
            </p>
          </div>
        </div>
        <Select value={status} onValueChange={(v) => setStatus(v ?? 'pending')}>
          <SelectTrigger className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {approvals.length === 0 ? (
        status === 'pending' ? (
          <EmptyState
            icon={<GavelIcon className="size-5" />}
            title="Nothing pending"
            description="No deploy or promote is waiting on approval right now."
          />
        ) : (
          <EmptyState
            icon={<GavelIcon className="size-5" />}
            title="No approvals match this filter"
            description={`No ${status} approvals to show.`}
            action={
              <Button
                size="sm"
                variant="outline"
                onClick={() => setStatus('pending')}
              >
                Clear filter
              </Button>
            }
          />
        )
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Service</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Image</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Requested by</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {approvals.map((a) => (
              <TableRow key={a.id}>
                <TableCell>
                  <Link
                    to="/apps/$name/overview"
                    params={{ name: a.service_name }}
                    className="text-foreground underline-offset-2 hover:underline"
                  >
                    {a.service_name}
                  </Link>
                  {a.source_service_name ? (
                    <span className="ml-1 text-xs text-muted-foreground">
                      from {a.source_service_name}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell className="capitalize">{a.action}</TableCell>
                <TableCell className="font-mono text-xs">{a.image}</TableCell>
                <TableCell>
                  <Badge variant={statusBadgeVariant(a.status)}>
                    {a.status}
                  </Badge>
                </TableCell>
                <TableCell>{a.requested_by_name}</TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {formatDate(a.created_at)}
                </TableCell>
                <TableCell className="text-right">
                  {a.status === 'pending' ? (
                    <div className="flex justify-end gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={actingID === a.id}
                        onClick={() => handleReject(a.id)}
                      >
                        Reject
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        disabled={actingID === a.id}
                        onClick={() =>
                          handleApprove(a.id, a.image, a.service_name)
                        }
                      >
                        Approve
                      </Button>
                    </div>
                  ) : (
                    <span className="text-xs text-muted-foreground">
                      {a.decided_at
                        ? `by ${a.approved_by_name ?? 'unknown'}`
                        : ''}
                    </span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {approve.isError || reject.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {(approve.error ?? reject.error)?.message}
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}

function DeployApprovalsPending() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Deploy approvals
        </h1>
      </div>
      <TableSkeleton columnCount={7} rowCount={5} />
    </div>
  )
}
