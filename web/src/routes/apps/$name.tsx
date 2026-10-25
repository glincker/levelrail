import { NodeOfflineBanner } from '../../components/NodeOfflineBanner'
import { StuckPanel } from '../../components/StuckPanel'
import { DelayedFallback } from '../../components/ui/delayed-fallback'
import { Suspense, useState } from 'react'
import { createFileRoute, Outlet, useRouterState } from '@tanstack/react-router'
import { appDetailQueryOptions, useApp } from '../../queries/apps'
import {
  deployStatusQueryOptions,
  useDeployStatus,
} from '../../queries/deploys'
import { summarizeAppStatus } from '../../lib/appStatus'
import { useAppAsleep } from '../../queries/appSleep'
import { Breadcrumbs } from '../../components/Breadcrumbs'
import { AppActionBar } from '../../components/apps/AppActionBar'
import { AppTabStrip } from '../../components/apps/AppTabStrip'
import { AppPaletteActions } from '../../components/apps/AppPaletteActions'
import { parseAppAction, type AppPaletteAction } from '../../lib/paletteAction'
import { AppHeaderBadges } from '../../components/apps/AppHeaderBadges'
import {
  DeploySheet,
  type DeployTab,
} from '../../components/overview/DeploySheet'
import { PendingDeployApprovalBanner } from '../../components/PendingDeployApprovalBanner'
import {
  PublishLockBanner,
  PublishLockToggle,
} from '../../components/PublishLockControls'
import { AppMaintenanceBanner } from '../../components/maintenance/AppMaintenanceBanner'
import { FixReturnBanner } from '../../components/deployDetail/FixReturnBanner'
import { TrialAppBanner } from '../../components/TrialAppBanner'
import { resourceErrorComponent } from '@/components/resourceErrorComponent'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { APP_SECTION_HELP } from '@/components/apps/appSectionHelp'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'

// Matches the 18 real section routes under /apps/$name/* (see
// AppScopedSidebar.tsx's own nav, the source of truth for these labels):
// the breadcrumb's trailing "sub-page" segment, keyed by the pathname's
// last segment. /overview is included even though it's the default
// redirect target, so the breadcrumb still names the page you're on.
const APP_SECTION_LABELS: Record<string, string> = {
  overview: 'Overview',
  deploys: 'Deploys',
  domains: 'Domains',
  'go-live': 'Go live',
  network: 'Network',
  services: 'Services',
  environment: 'Environment',
  source: 'Source',
  previews: 'Previews',
  'deploy-settings': 'Deploy settings',
  health: 'Health',
  resources: 'Resources',
  volumes: 'Volumes',
  integrations: 'Integrations',
  'scheduled-tasks': 'Jobs',
  'feature-flags': 'Feature flags',
  'split-testing': 'Split testing',
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
  validateSearch: (
    search: Record<string, unknown>,
  ): { action?: AppPaletteAction } => {
    const action = parseAppAction(search.action)
    return action ? { action } : {}
  },
  component: AppDetailLayout,
  pendingComponent: AppDetailLayoutSkeleton,
  errorComponent: resourceErrorComponent('app'),
})

function AppDetailLayout() {
  const { name } = Route.useParams()
  const { action } = Route.useSearch()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)
  const asleep = useAppAsleep(name)
  const status = summarizeAppStatus(conditions, asleep)

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
  const { t } = useTranslation('common')
  const [deployTab, setDeployTab] = useState<DeployTab | null>(null)
  const section = useRouterState({
    select: (s) => s.location.pathname.split('/').filter(Boolean).pop(),
  })
  const helpPath = section ? APP_SECTION_HELP[section] : undefined
  if (isViewingDeployLogs) {
    return <Outlet />
  }

  const breadcrumb = (
    <Breadcrumbs
      projectId={app.project_id}
      environmentId={app.environment_id}
      appName={app.name}
      page={section ? APP_SECTION_LABELS[section] : undefined}
    />
  )
  const banners = (
    <>
      {app.is_trial ? <TrialAppBanner app={app} /> : null}
      <NodeOfflineBanner conditions={conditions} nodeName={app.node_name} />
      <PendingDeployApprovalBanner appName={app.name} />
      <PublishLockBanner appName={app.name} />
      <AppMaintenanceBanner appName={app.name} />
      <FixReturnBanner appName={app.name} image={app.image} />
    </>
  )
  const content = (
    <>
      <DeploySheet
        appName={app.name}
        tab={deployTab}
        onClose={() => {
          setDeployTab(null)
        }}
      />
      <AppPaletteActions app={app} action={action} onDeploy={setDeployTab} />
      <StuckPanel kind="app" name={app.name} />
      <AppTabStrip name={app.name} />
      <Suspense fallback={<DelayedFallback />}>
        <Outlet />
      </Suspense>
    </>
  )

  if (isOverview) {
    return (
      <div className="space-y-6">
        {breadcrumb}
        {banners}
        {content}
      </div>
    )
  }

  return (
    <PageTemplate
      title={app.name}
      breadcrumb={breadcrumb}
      status={<Badge variant={status.variant}>{status.label}</Badge>}
      actions={
        <>
          <AppHeaderBadges app={app} />
          <span
            aria-hidden="true"
            className="hidden h-5 w-px bg-border lg:block"
          />
          <div className="flex items-center gap-2">
            <PublishLockToggle appName={app.name} />
            <AppActionBar app={app} onDeploy={setDeployTab} />
          </div>
        </>
      }
      helpPath={helpPath}
      helpLabel={
        helpPath
          ? t('appSectionHelp', {
              defaultValue: 'Read the guide for this page',
            })
          : undefined
      }
      banners={banners}
    >
      {content}
    </PageTemplate>
  )
}

// Mirrors the layout's own header (breadcrumb, name/badge/actions) plus a
// generic content block, since which section route is loading underneath
// isn't known yet.
function AppDetailLayoutSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Skeleton className="h-4 w-56" />

      <div className="flex flex-wrap items-center justify-between gap-3">
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
