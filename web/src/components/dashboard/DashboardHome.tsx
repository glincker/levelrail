import { Suspense, lazy, useEffect, useState } from 'react'
import { CaretDownIcon, RocketLaunchIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/kit'
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
import { StatusHeader } from './StatusHeader'
import { FleetTiles } from './FleetTiles'
import { NeedsAttention } from './NeedsAttention'
import { RecentActivity } from './RecentActivity'
import { QuickStart } from './QuickStart'

const RecentAlertsCard = lazy(() =>
  import('../RecentAlertsCard').then((m) => ({ default: m.RecentAlertsCard })),
)

function ResourceUsage({ apps }: { apps: AppListEntry[] }) {
  const [open, setOpen] = useState(false)
  return (
    <section aria-label="Resource usage" className="space-y-3">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => {
          setOpen((v) => !v)
        }}
        className="flex items-center gap-1.5 text-sm font-semibold text-foreground"
      >
        Resource usage
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
          title={`Welcome to ${brand.Name}`}
          description="Deploy your first app in under a minute."
          action={
            <CreateResourceWizard
              trigger={<Button size="lg">Deploy your first app</Button>}
            />
          }
        />
        <QuickStart hasApps={false} />
      </div>
    )
  }

  return (
    <div className="space-y-8">
      <StatusHeader firstAppName={apps[0]?.name} showSetup={isRoot} />
      <FleetTiles apps={apps} />
      <NeedsAttention />
      <Suspense fallback={null}>
        <RecentAlertsCard />
      </Suspense>
      <RecentActivity apps={apps} />
      <QuickStart hasApps />
      <ResourceUsage apps={apps} />
    </div>
  )
}
