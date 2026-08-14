import {
  createFileRoute,
  Link,
  Outlet,
  useRouterState,
} from '@tanstack/react-router'
import { appDetailQueryOptions, useApp } from '../../queries/apps'
import {
  deployStatusQueryOptions,
  useDeployStatus,
} from '../../queries/deploys'
import { summarizeAppStatus } from '../../lib/appStatus'
import { DeleteAppDialog } from '../../components/DeleteAppDialog'
import { DeployTriggerForm } from '../../components/DeployTriggerForm'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'

// App detail layout route (TASKS.md 1.10, expanded into a per-section
// nested-route layout): this file used to render all 8 sections itself as client-side
// Tabs/TabsContent panels. They are now real nested routes under
// /apps/$name/* (overview.tsx, domains.tsx, environment.tsx, health.tsx,
// resources.tsx, metrics.tsx, logs.tsx, alerts.tsx), each deep-linkable,
// matching the precedent /apps/$name/deploys/$deployId/logs already set
// as a real nested route rather than in-page tab state. This file is now
// the layout: it owns the loader (both queries below), the shared page
// header (app name, status, delete action), and the pinned deploy-trigger
// form, then renders <Outlet /> for whichever section route is active.
//
// Both queries are primed here, matching frontend-plan.md section 3's
// "cross-cutting" rule that a route's data comes from typed loaders, not
// fetches in the component body: the app resource itself (GET
// /api/v1/apps/{name}) and its current reconcile status (GET
// /api/v1/apps/{name}/deploys). Every child section route reads the same
// cache via useApp/useDeployStatus (keyed identically, see queries/apps.ts
// and queries/deploys.ts), so navigating between sections never re-fetches
// either query, the same sharing precedent AppScopedSidebar.tsx relies on
// to render the app name/status in the sidebar without its own fetch.
export const Route = createFileRoute('/apps/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(appDetailQueryOptions(name)),
      queryClient.ensureQueryData(deployStatusQueryOptions(name)),
    ]),
  component: AppDetailLayout,
  errorComponent: AppDetailError,
})

function AppDetailLayout() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)
  const status = summarizeAppStatus(conditions)

  // /apps/$name/deploys/$deployId/logs is a real, distinct nested route
  // (a sibling of the 8 section routes, one directory deeper). It renders
  // its own full header and full-height log pane (a focused build-log
  // view, not a panel meant to sit inside this layout's header/deploy-
  // trigger chrome), so when that child route is active this layout steps
  // aside entirely rather than wrapping it. There is no in-app link to
  // this route yet: the backend has no deploy-history/attempt-listing
  // endpoint to source a deployId from, so this only fixes the route for
  // direct navigation, see TASKS-v2.md.
  const isViewingDeployLogs = useRouterState({
    select: (s) => s.location.pathname.includes('/deploys/'),
  })
  if (isViewingDeployLogs) {
    return <Outlet />
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h1 className="text-lg font-semibold text-foreground">{app.name}</h1>
          <Badge variant={status.variant}>{status.label}</Badge>
        </div>
        <DeleteAppDialog name={app.name} />
      </div>

      {/* Deploy is the single most common action on this page, so it is
          pinned above the section outlet rather than scoped inside one
          section: it applies to the app as a whole, not to one config
          concern. */}
      <DeployTriggerForm appName={app.name} />

      <Outlet />
    </div>
  )
}

// fetchApp (queries/apps.ts) throws a plain Error for a 404, which lands
// here rather than crashing the whole route tree. Kept deliberately
// minimal: a name, a message, and a way back to the list.
function AppDetailError({ error }: { error: Error }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{error.message}</p>
        <Link to="/apps" className="mt-2 inline-block underline">
          Back to apps
        </Link>
      </AlertDescription>
    </Alert>
  )
}
