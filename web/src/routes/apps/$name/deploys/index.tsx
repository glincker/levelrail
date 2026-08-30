import { createFileRoute } from '@tanstack/react-router'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../../queries/deployAttempts'
import { useDeployStatus } from '../../../../queries/deploys'
import { DeployAttemptsList } from '../../../../components/DeployAttemptsList'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// A 9th app-scoped section, matching the 8 existing ones' "own loader,
// own deep-linkable route" shape (see routes/apps/$name.tsx's own doc
// comment): deploy history is a first-class, non-trivial concern (real
// row-per-attempt history, a log viewer per attempt, a rollback action),
// not something to squeeze onto Overview alongside the current-status
// ConditionsPanel it already renders. AppScopedSidebar.tsx links here.
//
// $name/deploys/$deployId/logs.tsx already exists one directory deeper
// as a real nested route; this index route is this task's own fix for
// that route's own doc comment ("There is no in-app link to this route
// yet: the backend has no deploy-history/attempt-listing endpoint to
// source a deployId from"): this page is exactly that source.
export const Route = createFileRoute('/apps/$name/deploys/')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: DeploysSection,
  pendingComponent: DeploysSectionPending,
})

function DeploysSection() {
  const { name } = Route.useParams()
  const { data: attempts } = useDeployAttempts(name)
  const { data: conditions } = useDeployStatus(name)

  return (
    <DeployAttemptsList appName={name} attempts={attempts} conditions={conditions} />
  )
}

// Route-level fallback for the loader's pending phase, matching
// DeployAttemptsList's own Card chrome so the skeleton doesn't jump when
// real rows swap in.
function DeploysSectionPending() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Deploy history</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4" aria-hidden="true">
        {Array.from({ length: 4 }, (_, i) => (
          <div key={i} className="flex items-start gap-3">
            <Skeleton className="mt-0.5 h-5 w-20 shrink-0 rounded-full" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-3 w-1/3" />
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
