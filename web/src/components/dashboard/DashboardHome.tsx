import { Suspense, lazy, useState } from 'react'
import { SignInHomeCards } from '../signin/SignInHomeCards'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import type { AppListEntry } from '../../types/appDetail'
import type { AppsSummary } from '../../queries/appPages'
import { appListQueryOptions } from '../../queries/apps'
import { useIsRoot } from '../../hooks/useIsRoot'
import { FleetUtilizationSummary } from '../FleetUtilizationSummary'
import { TopResourceConsumers } from '../TopResourceConsumers'
import { DashboardHero } from './DashboardHero'
import { DashboardBanners } from './DashboardBanners'
import { FleetTiles } from './FleetTiles'
import { NextSteps } from './NextSteps'
import { RecentActivity } from './RecentActivity'
import { QuickStart } from './QuickStart'
import { ResourcesSection } from './ResourcesSection'
import { RecommendationsLine } from './RecommendationsLine'
import { ReservedUsageCard } from './ReservedUsageCard'
import { ResourceAdvisorTile } from '../resourceAdvisor/ResourceAdvisorTile'
import { StorageUsageCard } from './StorageUsageCard'
import { SleepFleetCard } from './SleepFleetCard'
import { PlatformCapabilities } from './PlatformCapabilities'

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

export function DashboardHome({
  summary,
  sample,
}: {
  summary: AppsSummary
  sample: AppListEntry[]
}) {
  const isRoot = useIsRoot()
  const [deployStarted, setDeployStarted] = useState(false)
  const [firstRunDone, setFirstRunDone] = useState(false)
  const hasApps = summary.total > 0

  if (!hasApps || (deployStarted && !firstRunDone)) {
    return (
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
  }

  return (
    <DashboardHero summary={summary} banners={<DashboardBanners />}>
      <SignInHomeCards />
      <FleetTiles summary={summary} sample={sample} />
      <div className="@container">
        <div className="grid gap-4 @4xl:grid-cols-2">
          <NextSteps summary={summary} sample={sample} />
          <RecentActivity sample={sample} />
        </div>
      </div>
      <ResourcesSection />
      <ResourceAdvisorTile />
      <Suspense fallback={null}>
        <RecentAlertsCard />
      </Suspense>
      <RecommendationsLine showSetup={false} />
      <UsageDetails />
    </DashboardHero>
  )
}
