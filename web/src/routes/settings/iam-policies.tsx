import { createFileRoute } from '@tanstack/react-router'
import { IamWorkspace } from '../../components/iam/IamWorkspace'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { policyListQueryOptions } from '../../queries/iamPolicies'

// Access policy workspace: guided builder, simulator, analyzer and
// attachments over /api/v1/iam. Account-level, so it lives under
// routes/settings/ next to tokens.tsx. The loader primes the policy list
// before render.
export const Route = createFileRoute('/settings/iam-policies')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(policyListQueryOptions()),
  component: IamWorkspace,
  pendingComponent: () => <TableSkeleton columnCount={4} />,
})
