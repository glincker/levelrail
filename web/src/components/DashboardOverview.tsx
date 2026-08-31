import type { ReactNode } from 'react'
import { useEffect } from 'react'
import { Link } from '@tanstack/react-router'
import {
  PackageIcon,
  CheckCircleIcon,
  WarningIcon,
  ArrowsClockwiseIcon,
  SparkleIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { AppListEntry } from '../types/appDetail'
import { useBrand } from '../hooks/useBrand'
import { STATUS_DOT_COLOR } from '../lib/appStatus'
import { CreateResourceWizard } from './CreateResourceWizard'
import { OnboardingFlow } from './OnboardingFlow'
import { useCompleteOnboarding } from '../queries/onboarding'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

// GET /api/v1/apps carries no updated/last-deploy timestamp, so this
// keeps the API's own ordering rather than sorting by recency.
const APP_ROW_CAP = 6
const ATTENTION_ROW_CAP = 5

function bucketApps(apps: AppListEntry[]) {
  const healthy = apps.filter((a) => a.status.variant === 'success')
  const attention = apps.filter((a) => a.status.variant === 'destructive')
  const building = apps.filter((a) => a.status.variant === 'muted')
  return { healthy, attention, building }
}

export function DashboardOverview({
  apps,
  onboardingCompleted,
}: {
  apps: AppListEntry[]
  onboardingCompleted: boolean
}) {
  // Belt-and-suspenders: a first app created some way other than the
  // onboarding checklist's own "Deploy an app" button (the CLI, the API
  // directly, or the ordinary "New app" button) still needs to mark
  // onboarding complete, so the checklist doesn't reappear if every app
  // later gets deleted back down to zero. OnboardingFlow's own "Skip
  // setup" button covers the other way this flag gets set.
  const completeOnboarding = useCompleteOnboarding()
  const hasApps = apps.length > 0
  const { mutate: markOnboardingComplete } = completeOnboarding
  useEffect(() => {
    if (hasApps && !onboardingCompleted) {
      markOnboardingComplete()
    }
  }, [hasApps, onboardingCompleted, markOnboardingComplete])

  if (apps.length === 0) {
    return onboardingCompleted ? <WelcomeEmptyState /> : <OnboardingFlow />
  }

  const { healthy, attention, building } = bucketApps(apps)

  return (
    <div className="space-y-6">
      <div className="flex items-baseline justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">Dashboard</h1>
        <CreateResourceWizard
          trigger={
            <Button size="sm">
              <PlusIcon />
              New app
            </Button>
          }
        />
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="Total apps" value={apps.length} icon={<PackageIcon />} />
        <StatCard
          label="Healthy"
          value={healthy.length}
          icon={<CheckCircleIcon />}
          tone="success"
        />
        <StatCard
          label="Attention needed"
          value={attention.length}
          icon={<WarningIcon />}
          tone="destructive"
        />
        <StatCard
          label="Building"
          value={building.length}
          icon={<ArrowsClockwiseIcon />}
          tone="muted"
        />
      </div>

      {attention.length > 0 ? (
        <section>
          <h2 className="mb-2 text-sm font-medium text-foreground">
            Needs attention
          </h2>
          <div className="divide-y divide-border rounded-lg border border-border bg-card">
            {attention.slice(0, ATTENTION_ROW_CAP).map((app) => (
              <DashboardAppRow key={app.name} app={app} />
            ))}
          </div>
          {attention.length > ATTENTION_ROW_CAP ? (
            <p className="mt-1.5 text-xs text-muted-foreground">
              and {attention.length - ATTENTION_ROW_CAP} more
            </p>
          ) : null}
        </section>
      ) : null}

      <section>
        <div className="mb-2 flex items-center justify-between">
          <h2 className="text-sm font-medium text-foreground">Apps</h2>
          <Link
            to="/apps"
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            View all
          </Link>
        </div>
        <div className="divide-y divide-border rounded-lg border border-border bg-card">
          {apps.slice(0, APP_ROW_CAP).map((app) => (
            <DashboardAppRow key={app.name} app={app} />
          ))}
        </div>
      </section>
    </div>
  )
}

const STAT_TONE_CLASS: Record<'default' | 'success' | 'destructive' | 'muted', string> = {
  default: 'bg-muted text-muted-foreground',
  success: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  destructive: 'bg-destructive/10 text-destructive dark:bg-destructive/20',
  muted: 'bg-muted text-muted-foreground',
}

function StatCard({
  label,
  value,
  icon,
  tone = 'default',
}: {
  label: string
  value: number
  icon: ReactNode
  tone?: 'default' | 'success' | 'destructive' | 'muted'
}) {
  return (
    <Card size="sm">
      <CardContent className="flex items-center gap-3">
        <span
          className={`flex size-9 shrink-0 items-center justify-center rounded-full ${STAT_TONE_CLASS[tone]}`}
          aria-hidden="true"
        >
          {icon}
        </span>
        <div className="min-w-0">
          <p className="text-xl font-semibold text-foreground">{value}</p>
          <p className="truncate text-xs text-muted-foreground">{label}</p>
        </div>
      </CardContent>
    </Card>
  )
}

function DashboardAppRow({ app }: { app: AppListEntry }) {
  return (
    <Link
      to="/apps/$name"
      params={{ name: app.name }}
      className="flex items-center gap-3 px-4 py-2.5 text-sm transition-colors hover:bg-muted/60"
    >
      {/* Decorative: the Badge below already carries the status as
          accessible text, so this dot (unlike AppRow's StatusDot) skips
          role="img"/aria-label to avoid announcing it twice. */}
      <span
        className={`size-2 shrink-0 rounded-full ${STATUS_DOT_COLOR[app.status.variant]}`}
        aria-hidden="true"
      />
      <span className="min-w-0 flex-1 truncate font-medium text-foreground">
        {app.name}
      </span>
      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {app.image}
      </span>
      <Badge variant={app.status.variant} className="shrink-0">
        {app.status.label}
      </Badge>
    </Link>
  )
}

function WelcomeEmptyState() {
  const brand = useBrand()
  return (
    <div className="flex flex-col items-center justify-center gap-4 rounded-lg border border-dashed border-border bg-card/50 px-6 py-24 text-center">
      <span
        className="flex size-14 items-center justify-center rounded-full bg-primary/10 text-primary"
        aria-hidden="true"
      >
        <SparkleIcon className="size-7" />
      </span>
      <div className="space-y-1.5">
        <h1 className="text-xl font-semibold text-foreground">
          Welcome to {brand.Name}
        </h1>
        <p className="mx-auto max-w-md text-sm text-muted-foreground">
          Deploy your first app to get started: push to a connected git repo,
          or create one directly if you already have a built image.
        </p>
      </div>
      <CreateResourceWizard
        trigger={
          <Button>
            <PlusIcon />
            Create app
          </Button>
        }
      />
    </div>
  )
}
