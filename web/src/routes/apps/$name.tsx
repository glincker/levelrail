import { useState } from 'react'
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
import { routeErrorMessage } from '../../lib/apiError'
import { Breadcrumbs } from '../../components/Breadcrumbs'
import { AppActionBar } from '../../components/apps/AppActionBar'
import {
  DeploySheet,
  type DeployTab,
} from '../../components/overview/DeploySheet'
import { PendingDeployApprovalBanner } from '../../components/PendingDeployApprovalBanner'
import { TrialAppBanner } from '../../components/TrialAppBanner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// Matches the 18 real section routes under /apps/$name/* (see
// AppScopedSidebar.tsx's own nav, the source of truth for these labels):
// the breadcrumb's trailing "sub-page" segment, keyed by the pathname's
// last segment. /overview is included even though it's the default
// redirect target, so the breadcrumb still names the page you're on.
const APP_SECTION_LABELS: Record<string, string> = {
  overview: 'Overview',
  deploys: 'Deploys',
  domains: 'Domains',
  network: 'Network',
  services: 'Services',
  environment: 'Environment',
  source: 'Source',
  'deploy-settings': 'Deploy settings',
  health: 'Health',
  resources: 'Resources',
  volumes: 'Volumes',
  integrations: 'Integrations',
  'scheduled-tasks': 'Scheduled tasks',
  'feature-flags': 'Feature flags',
  metrics: 'Metrics',
  logs: 'Logs',
  alerts: 'Alerts',
  exec: 'Exec',
}

// Layout for /apps/$name/*: owns the loader, the page header with its action
// bar, and the deploy sheet, then renders the active section via <Outlet />.
// Section routes share this cache through useApp/useDeployStatus.
export const Route = createFileRoute('/apps/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(appDetailQueryOptions(name)),
      queryClient.ensureQueryData(deployStatusQueryOptions(name)),
    ]),
  component: AppDetailLayout,
  pendingComponent: AppDetailLayoutSkeleton,
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
  // direct navigation.
  const isViewingDeployLogs = useRouterState({
    select: (s) => s.location.pathname.includes('/deploys/'),
  })
  const isOverview = useRouterState({
    select: (s) => s.location.pathname.endsWith('/overview'),
  })
  const [deployTab, setDeployTab] = useState<DeployTab | null>(null)
  const section = useRouterState({
    select: (s) => s.location.pathname.split('/').filter(Boolean).pop(),
  })
  if (isViewingDeployLogs) {
    return <Outlet />
  }

  return (
    <div className="space-y-6">
      <div>
        <Breadcrumbs
          projectId={app.project_id}
          environmentId={app.environment_id}
          appName={app.name}
          page={section ? APP_SECTION_LABELS[section] : undefined}
        />
      </div>
      {isOverview ? null : (
        <PageHeader
          title={app.name}
          status={<Badge variant={status.variant}>{status.label}</Badge>}
          actions={<AppActionBar app={app} onDeploy={setDeployTab} />}
        />
      )}

      {app.is_trial ? <TrialAppBanner name={app.name} /> : null}

      <PendingDeployApprovalBanner appName={app.name} />

      <DeploySheet
        appName={app.name}
        tab={deployTab}
        onClose={() => {
          setDeployTab(null)
        }}
      />

      <Outlet />
    </div>
  )
}

// Mirrors the layout's own header (breadcrumb, name/badge/actions) plus a
// generic content block, since which section route is loading underneath
// isn't known yet.
function AppDetailLayoutSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Skeleton className="h-4 w-56" />

      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
        <div className="flex items-center gap-2">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-8 w-8 rounded-md" />
          ))}
        </div>
      </div>

      <div className="space-y-3 rounded-lg border border-border p-4">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-2/3" />
      </div>
    </div>
  )
}

// fetchApp (queries/apps.ts) throws a plain Error for a 404, which lands
// here rather than crashing the whole route tree. Kept deliberately
// minimal: a name, a message, and a way back to the list.
function AppDetailError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link to="/apps" className="mt-2 inline-block underline">
          Back to apps
        </Link>
      </AlertDescription>
    </Alert>
  )
}
