import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { policyListQueryOptions } from '../../queries/iamPolicies'
import { PolicyTable } from '../../components/PolicyTable'
import { PolicyFormDialog } from '../../components/PolicyFormDialog'
import { Button } from '@/components/ui/button'
import { TableSkeleton } from '@/components/ui/table-skeleton'

// Dashboard UI for the IAM policy engine (internal/iam,
// levelrail-cli iam policies create/list/get/update/delete/attach/detach/
// attachments): a policy is a JSON document of Allow/Deny statements,
// attachable to a user or an API token, additive on top of the existing
// flat abilities list. Account-level, not scoped to one app, so it lives
// under routes/settings/ next to tokens.tsx and cli-access.tsx.
// Typed loader primes the Query cache before render, same pattern those
// two routes already established.
export const Route = createFileRoute('/settings/iam-policies')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(policyListQueryOptions()),
  component: IamPoliciesPage,
  pendingComponent: IamPoliciesPending,
})

function IamPoliciesPage() {
  const { data: policies } = useSuspenseQuery(policyListQueryOptions())

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <ShieldCheckIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              IAM policies
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              A policy is additive on top of a token or user&apos;s existing
              abilities: an explicit Deny always overrides, an explicit
              Allow can grant access narrower than a token&apos;s global
              scope without widening it.
            </p>
          </div>
        </div>
        <PolicyFormDialog trigger={<Button>Create policy</Button>} />
      </div>
      <PolicyTable policies={policies} />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// PolicyTable's own 4-column shape so the skeleton doesn't jump when
// real rows swap in.
function IamPoliciesPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">IAM policies</h1>
      <TableSkeleton columnCount={4} />
    </div>
  )
}
