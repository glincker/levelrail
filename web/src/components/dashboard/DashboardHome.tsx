import { Suspense, lazy, useEffect, useState } from 'react'
import { CaretDownIcon, RocketLaunchIcon } from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/ui/empty-state'
import { Button } from '@/components/ui/button'
import type { AppListEntry } from '../../types/appDetail'
import { useCompleteOnboarding } from '../../queries/onboarding'
import type { OnboardingState } from '../../queries/onboarding'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useBrand } from '../../hooks/useBrand'
import { SetupWizard } from '../setup/SetupWizard'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { FleetResourceChart } from '../FleetResourceChart'
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
import { PlatformCapabilities } from './PlatformCapabilities'

const RecentAlertsCard = lazy(() =>
  import('../RecentAlertsCard').then((m) => ({ default: m.RecentAlertsCard })),
)

function UsageDetails({ apps }: { apps: AppListEntry[] }) {
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
          <FleetResourceChart />
          <TopResourceConsumers apps={apps} />
          <ReservedUsageCard apps={apps} />
          <StorageUsageCard />
        </div>
      ) : null}
    </section>
  )
}

export function DashboardHome({
  apps,
  onboarding,
}: {
  apps: AppListEntry[]
  onboarding: OnboardingState
}) {
  const brand = useBrand()
  const { t } = useTranslation('dashboard')
  const isRoot = useIsRoot()
  const completeOnboarding = useCompleteOnboarding()
  const hasApps = apps.length > 0
  const wizardStarted = onboarding.current_step !== ''
  const { mutate: markOnboardingComplete } = completeOnboarding
  useEffect(() => {
    if (hasApps && !onboarding.completed && !wizardStarted) {
      markOnboardingComplete()
    }
  }, [hasApps, onboarding.completed, wizardStarted, markOnboardingComplete])

  if (isRoot && !onboarding.completed && (!hasApps || wizardStarted)) {
    return <SetupWizard />
  }

  if (!hasApps) {
    return (
      <div className="space-y-8">
        <EmptyState
          illustration="rocket"
          icon={<RocketLaunchIcon className="size-6" />}
          title={t('welcome.title', { brand: brand.Name })}
          description={t('welcome.description')}
          action={
            <CreateResourceWizard
              trigger={<Button size="lg">{t('welcome.cta')}</Button>}
            />
          }
        />
        <RecommendationsLine showSetup={isRoot} />
        <QuickStart />
        <PlatformCapabilities />
      </div>
    )
  }

  return (
    <DashboardHero apps={apps} banners={<DashboardBanners />}>
      <FleetTiles apps={apps} />
      <div className="@container">
        <div className="grid gap-4 @4xl:grid-cols-2">
          <NextSteps apps={apps} />
          <RecentActivity apps={apps} />
        </div>
      </div>
      <ResourcesSection />
      <ResourceAdvisorTile />
      <Suspense fallback={null}>
        <RecentAlertsCard />
      </Suspense>
      <RecommendationsLine showSetup={false} />
      <UsageDetails apps={apps} />
    </DashboardHero>
  )
}
