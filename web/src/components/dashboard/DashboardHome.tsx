import { Suspense, lazy, useState, type ReactNode } from 'react'
import { SignInHomeCards } from '../signin/SignInHomeCards'
import {
  CaretDownIcon,
  SlidersHorizontalIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import type { AppListEntry } from '../../types/appDetail'
import type { AppsSummary } from '../../queries/appPages'
import { appListQueryOptions } from '../../queries/apps'
import { anyAppsQueryOptions } from '../../queries/appPages'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useDashboardLayout } from '../../hooks/useDashboardLayout'
import {
  HALF_WIDTH,
  visibleOrder,
  type WidgetId,
} from '../../lib/dashboardLayout'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { FleetUtilizationSummary } from '../FleetUtilizationSummary'
import { TopResourceConsumers } from '../TopResourceConsumers'
import { DashboardHero } from './DashboardHero'
import { FleetTiles } from './FleetTiles'
import { NextSteps } from './NextSteps'
import { RecentActivity } from './RecentActivity'
import { AppsPanel } from './AppsPanel'
import { CopyRetryDialog } from './CopyRetryDialog'
import { StatusStrip } from './StatusStrip'
import { WidgetFrame } from './WidgetFrame'
import { QuickStart } from './QuickStart'
import { ResourcesSection } from './ResourcesSection'
import { RecommendationsLine } from './RecommendationsLine'
import { ReservedUsageCard } from './ReservedUsageCard'
import { ResourceAdvisorTile } from '../resourceAdvisor/ResourceAdvisorTile'
import { StorageUsageCard } from './StorageUsageCard'
import { SleepFleetCard } from './SleepFleetCard'
import { PlatformCapabilities } from './PlatformCapabilities'
import { DashboardViews } from './DashboardViews'

const FirstDeploy = lazy(() =>
  import('../getStarted/FirstDeploy').then((m) => ({
    default: m.FirstDeploy,
  })),
)

const FleetResourceChart = lazy(() =>
  import('../FleetResourceChart').then((m) => ({
    default: m.FleetResourceChart,
  })),
)

const RecentAlertsCard = lazy(() =>
  import('../RecentAlertsCard').then((m) => ({ default: m.RecentAlertsCard })),
)

function UsageApps() {
  const { data: apps = [] } = useQuery(appListQueryOptions())
  return (
    <>
      <TopResourceConsumers apps={apps} />
      <ReservedUsageCard apps={apps} />
    </>
  )
}

function UsageDetails() {
  const { t } = useTranslation('dashboard')
  const [open, setOpen] = useState(false)
  return (
    <section aria-label={t('usageDetails.title')} className="space-y-3">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => {
          setOpen((v) => !v)
        }}
        className="flex items-center gap-1.5 text-sm font-semibold text-foreground"
      >
        {t('usageDetails.title')}
        <CaretDownIcon
          aria-hidden="true"
          className={`size-3.5 transition-transform duration-200 ${open ? 'rotate-180' : ''}`}
        />
      </button>
      {open ? (
        <div className="space-y-4">
          <FleetUtilizationSummary />
          <Suspense fallback={null}>
            <FleetResourceChart />
          </Suspense>
          <UsageApps />
          <StorageUsageCard />
          <SleepFleetCard />
        </div>
      ) : null}
    </section>
  )
}

function StartView({
  summary,
  sample,
}: {
  summary: AppsSummary
  sample: AppListEntry[]
}) {
  const { t } = useTranslation('dashboard')
  const { layout, move, toggle, reset } = useDashboardLayout()
  const [editing, setEditing] = useState(false)

  const widgets: Record<WidgetId, ReactNode> = {
    attention: <NextSteps summary={summary} sample={sample} />,
    apps: <AppsPanel />,
    activity: <RecentActivity sample={sample} />,
    fleet: <FleetTiles summary={summary} sample={sample} />,
    resources: <ResourcesSection />,
    advisor: <ResourceAdvisorTile />,
    alerts: (
      <Suspense fallback={null}>
        <RecentAlertsCard />
      </Suspense>
    ),
    usage: <UsageDetails />,
  }
  const shown = editing ? layout.order : visibleOrder(layout)

  return (
    <div className="flex flex-col gap-(--glinr-page-gap)">
      <StatusStrip
        apps={summary.total}
        trailing={
          <span className="flex items-center gap-1">
            {editing ? (
              <Button size="sm" variant="ghost" onClick={reset}>
                {t('customize.reset')}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant={editing ? 'default' : 'ghost'}
              aria-pressed={editing}
              aria-label={editing ? t('customize.done') : t('customize.edit')}
              onClick={() => {
                setEditing((v) => !v)
              }}
            >
              <SlidersHorizontalIcon />
              {editing ? t('customize.done') : null}
            </Button>
          </span>
        }
      />
      <SignInHomeCards />
      <div className="@container">
        <div className="grid gap-4 @4xl:grid-cols-2">
          {shown.map((id, index) => (
            <div
              key={id}
              className={cn(
                'min-w-0 space-y-2',
                !HALF_WIDTH.includes(id) && '@4xl:col-span-2',
                layout.hidden.includes(id) && 'opacity-40',
              )}
            >
              <WidgetFrame
                id={id}
                index={index}
                total={shown.length}
                editing={editing}
                onMove={move}
                onHide={toggle}
              >
                {widgets[id]}
              </WidgetFrame>
              {editing && layout.hidden.includes(id) ? (
                <Button
                  size="sm"
                  variant="outline"
                  className="mt-2"
                  onClick={() => {
                    toggle(id)
                  }}
                >
                  {t('customize.show', { name: t(`widgets.${id}`) })}
                </Button>
              ) : null}
            </div>
          ))}
        </div>
      </div>
      <RecommendationsLine showSetup={false} />
      <CopyRetryDialog />
    </div>
  )
}

export function DashboardHome({
  summary,
  sample,
  board,
}: {
  summary: AppsSummary
  sample: AppListEntry[]
  board?: string
}) {
  const isRoot = useIsRoot()
  const anyApps = useQuery(anyAppsQueryOptions())
  const [deployStarted, setDeployStarted] = useState(false)
  const [firstRunDone, setFirstRunDone] = useState(false)
  const hasApps = summary.total > 0 || anyApps.data === true

  if (summary.total === 0 && anyApps.isPending) {
    return <Skeleton className="h-64 w-full" />
  }

  if (!hasApps || (deployStarted && !firstRunDone)) {
    const firstRun = (
      <div className="space-y-8">
        <SignInHomeCards />
        <Suspense fallback={null}>
          <FirstDeploy
            onStarted={() => {
              setDeployStarted(true)
            }}
            onDone={() => {
              setFirstRunDone(true)
            }}
          />
        </Suspense>
        {hasApps ? null : (
          <>
            <RecommendationsLine showSetup={isRoot} />
            <QuickStart />
            <PlatformCapabilities />
          </>
        )}
      </div>
    )
    // Keep the board tabs reachable before the first app so Metrics and saved boards are not hidden.
    return <DashboardViews start={firstRun} hasApps={hasApps} board={board} />
  }

  return (
    <DashboardHero>
      <DashboardViews
        start={<StartView summary={summary} sample={sample} />}
        hasApps
        board={board}
      />
    </DashboardHero>
  )
}
