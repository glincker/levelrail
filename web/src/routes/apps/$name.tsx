import { createFileRoute, Link } from '@tanstack/react-router'
import { appDetailQueryOptions, useApp } from '../../queries/apps'
import {
  deployStatusQueryOptions,
  useDeployStatus,
} from '../../queries/deploys'
import { AppOverview } from '../../components/AppOverview'
import { ConditionsPanel } from '../../components/ConditionsPanel'
import { DeployTriggerForm } from '../../components/DeployTriggerForm'
import { DomainEditor } from '../../components/DomainEditor'
import { EnvEditor } from '../../components/EnvEditor'
import { SecretsEditor } from '../../components/SecretsEditor'

// App detail route (TASKS.md 1.10). Two queries are primed in the
// loader, matching frontend-plan.md section 3's "cross-cutting" rule
// that a route's data comes from typed loaders, not fetches in the
// component body: the app resource itself (GET /api/v1/apps/{name})
// and its current reconcile status (GET /api/v1/apps/{name}/deploys).
// Both are read via useSuspenseQuery under the hood (useApp,
// useDeployStatus), so the component below never renders a loading
// state of its own, the loader already guarantees the cache is warm.
export const Route = createFileRoute('/apps/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(appDetailQueryOptions(name)),
      queryClient.ensureQueryData(deployStatusQueryOptions(name)),
    ]),
  component: AppDetailPage,
  errorComponent: AppDetailError,
})

function AppDetailPage() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/apps"
          className="text-xs text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100"
        >
          &larr; Apps
        </Link>
        <h1 className="mt-1 text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          {app.name}
        </h1>
      </div>

      <AppOverview app={app} />
      <ConditionsPanel conditions={conditions} />
      <DeployTriggerForm appName={app.name} />
      <DomainEditor app={app} />
      <EnvEditor app={app} />
      <SecretsEditor appName={app.name} />
    </div>
  )
}

// fetchApp (queries/apps.ts) throws a plain Error for a 404, which lands
// here rather than crashing the whole route tree. Kept deliberately
// minimal: a name, a message, and a way back to the list.
function AppDetailError({ error }: { error: Error }) {
  return (
    <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800 dark:border-red-900 dark:bg-red-900/20 dark:text-red-300">
      <p>{error.message}</p>
      <Link to="/apps" className="mt-2 inline-block underline">
        Back to apps
      </Link>
    </div>
  )
}
