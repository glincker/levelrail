import {
  createFileRoute,
  Link,
  Outlet,
  useRouterState,
} from '@tanstack/react-router'
import {
  PulseIcon,
  ArrowLeftIcon,
  BellIcon,
  CpuIcon,
  GlobeIcon,
  HeartbeatIcon,
  SquaresFourIcon,
  ScrollIcon,
  BracketsCurlyIcon,
} from '@phosphor-icons/react/dist/ssr'
import { appDetailQueryOptions, useApp } from '../../queries/apps'
import {
  deployStatusQueryOptions,
  useDeployStatus,
} from '../../queries/deploys'
import type { ReconcileCondition } from '../../types/deploy'
import { AppOverview } from '../../components/AppOverview'
import { ConditionsPanel } from '../../components/ConditionsPanel'
import { DeleteAppDialog } from '../../components/DeleteAppDialog'
import { DeployTriggerForm } from '../../components/DeployTriggerForm'
import { DomainEditor } from '../../components/DomainEditor'
import { EnvEditor } from '../../components/EnvEditor'
import { HealthCheckEditor } from '../../components/HealthCheckEditor'
import { ResourceLimitsEditor } from '../../components/ResourceLimitsEditor'
import { SecretsEditor } from '../../components/SecretsEditor'
import { MetricsDashboard } from '../../components/MetricsDashboard'
import { LogSearchPanel } from '../../components/LogSearchPanel'
import { AlertRulesPanel } from '../../components/AlertRulesPanel'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { VariantProps } from 'class-variance-authority'

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

// Purely a display-layer summary of the same `conditions` array
// ConditionsPanel renders in full: no new fetch, no new business logic,
// just a one-line rollup for the page header so the operator does not
// have to open the Overview tab to learn whether the app is healthy.
// Mirrors how Coolify/Dokploy show a single status pill next to the
// resource name at the top of the detail page.
function summarizeConditions(conditions: ReconcileCondition[]): {
  label: string
  variant: VariantProps<typeof badgeVariants>['variant']
} {
  if (conditions.length === 0) {
    return { label: 'No status yet', variant: 'muted' }
  }
  if (conditions.some((c) => c.Status === 'False')) {
    return { label: 'Attention needed', variant: 'destructive' }
  }
  if (conditions.every((c) => c.Status === 'True')) {
    return { label: 'Healthy', variant: 'success' }
  }
  return { label: 'Reconciling', variant: 'muted' }
}

function AppDetailPage() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)
  const status = summarizeConditions(conditions)

  // /apps/$name/deploys/$deployId/logs is a real, distinct nested route
  // (routeTree.gen.ts: its getParentRoute is this route), but this
  // component previously never rendered an <Outlet />, so navigating
  // there rendered this page's own tab shell with nowhere for the child
  // route to mount, a silently broken URL. DeployLogsPage renders its
  // own full header and full-height log pane (a focused build-log view,
  // not a panel meant to sit inside these tabs), so when that child
  // route is active this page steps aside entirely rather than nesting
  // it under the tab UI. There is no in-app link to this route yet: the
  // backend has no deploy-history/attempt-listing endpoint to source a
  // deployId from (internal/api's own doc comment is explicit that
  // deploys.go returns current reconcile conditions, not an
  // attempt-by-attempt log), so this only fixes the route for direct
  // navigation, it does not add a "recent deploys" list. That's a real,
  // separate, store-schema-sized feature, not a wiring fix, see
  // TASKS-v2.md.
  const isViewingDeployLogs = useRouterState({
    select: (s) => s.location.pathname.includes('/deploys/'),
  })
  if (isViewingDeployLogs) {
    return <Outlet />
  }

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/apps"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" />
          Apps
        </Link>
        <div className="mt-1 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <h1 className="text-lg font-semibold text-foreground">
              {app.name}
            </h1>
            <Badge variant={status.variant}>{status.label}</Badge>
          </div>
          <DeleteAppDialog name={app.name} />
        </div>
      </div>

      {/* Deploy is the single most common action on this page, so it is
          pinned above the tab shell rather than scoped inside one tab:
          it applies to the app as a whole, not to one config concern. */}
      <DeployTriggerForm appName={app.name} />

      <Tabs defaultValue="overview">
        <TabsList variant="line" className="border-b border-border">
          <TabsTrigger value="overview">
            <SquaresFourIcon className="size-3.5" data-icon="inline-start" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="domains">
            <GlobeIcon className="size-3.5" data-icon="inline-start" />
            Domains
          </TabsTrigger>
          <TabsTrigger value="environment">
            <BracketsCurlyIcon className="size-3.5" data-icon="inline-start" />
            Environment
          </TabsTrigger>
          <TabsTrigger value="health">
            <HeartbeatIcon className="size-3.5" data-icon="inline-start" />
            Health
          </TabsTrigger>
          <TabsTrigger value="resources">
            <CpuIcon className="size-3.5" data-icon="inline-start" />
            Resources
          </TabsTrigger>
          <TabsTrigger value="metrics">
            <PulseIcon className="size-3.5" data-icon="inline-start" />
            Metrics
          </TabsTrigger>
          <TabsTrigger value="logs">
            <ScrollIcon className="size-3.5" data-icon="inline-start" />
            Logs
          </TabsTrigger>
          <TabsTrigger value="alerts">
            <BellIcon className="size-3.5" data-icon="inline-start" />
            Alerts
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4 space-y-6">
          <AppOverview app={app} />
          <ConditionsPanel conditions={conditions} />
        </TabsContent>

        <TabsContent value="domains" className="mt-4">
          <DomainEditor app={app} />
        </TabsContent>

        <TabsContent value="environment" className="mt-4 space-y-6">
          <EnvEditor app={app} />
          <SecretsEditor appName={app.name} />
        </TabsContent>

        <TabsContent value="health" className="mt-4">
          <HealthCheckEditor app={app} />
        </TabsContent>

        <TabsContent value="resources" className="mt-4">
          <ResourceLimitsEditor app={app} />
        </TabsContent>

        <TabsContent value="metrics" className="mt-4">
          <MetricsDashboard appName={app.name} conditions={conditions} />
        </TabsContent>

        <TabsContent value="logs" className="mt-4">
          <LogSearchPanel appName={app.name} />
        </TabsContent>

        <TabsContent value="alerts" className="mt-4">
          <AlertRulesPanel appName={app.name} />
        </TabsContent>
      </Tabs>
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
