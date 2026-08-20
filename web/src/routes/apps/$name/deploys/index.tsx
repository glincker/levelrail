import { createFileRoute } from '@tanstack/react-router'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../../queries/deployAttempts'
import { useDeployStatus } from '../../../../queries/deploys'
import { DeployAttemptsList } from '../../../../components/DeployAttemptsList'

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
})

function DeploysSection() {
  const { name } = Route.useParams()
  const { data: attempts } = useDeployAttempts(name)
  const { data: conditions } = useDeployStatus(name)

  return (
    <DeployAttemptsList appName={name} attempts={attempts} conditions={conditions} />
  )
}
