import { useState } from 'react'
import { Link, createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr'
import { deployAttemptsQueryOptions } from '../../../../queries/deployAttempts'
import { useDeployProgress } from '../../../../hooks/useDeployProgress'
import { useApp } from '../../../../queries/apps'
import { useAppTimeline } from '../../../../queries/appTimeline'
import { AppHistoryTimeline } from '../../../../components/deployTimeline/AppHistoryTimeline'
import { DeployAttemptsList } from '../../../../components/DeployAttemptsList'
import { ScheduledDeploysList } from '../../../../components/deploy/ScheduledDeploysList'
import { BuildLimitsCard } from '../../../../components/BuildLimitsCard'
import { BuildTrendCard } from '../../../../components/BuildTrendCard'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import {
  DeploySheet,
  type DeployTab,
} from '../../../../components/overview/DeploySheet'

// A 9th app-scoped section, matching the 8 existing ones' "own loader,
// own deep-linkable route" shape (see routes/apps/$name.tsx's own doc
// comment): deploy history is a first-class, non-trivial concern (real
// row-per-attempt history, a log viewer per attempt, a rollback action),
// not something to squeeze onto Overview alongside the current-status
// ConditionsPanel it already renders. AppScopedSidebar.tsx links here.
//
// $name/deploys/$deployId/logs.tsx already exists one directory deeper
// as a real nested route; this index route is this task's own fix for
// that route's own doc comment ("There is no in-app link to this route
// yet: the backend has no deploy-history/attempt-listing endpoint to
// source a deployId from"): this page is exactly that source.
export const Route = createFileRoute('/apps/$name/deploys/')({
  loader: async ({ context: { queryClient }, params: { name } }) => {
    await Promise.all([
      queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
    ])
  },
  component: DeploysSection,
  pendingComponent: DeploysSectionPending,
})

function DeploysSection() {
  const { name } = Route.useParams()
  const { t } = useTranslation('common')
  const { attempts, conditions } = useDeployProgress(name)
  const [tab, setTab] = useState<DeployTab | null>(null)

  const { data: app } = useApp(name)
  const { data: activity } = useAppTimeline(name)

  return (
    <div className="space-y-6">
      <AppHistoryTimeline
        appName={name}
        attempts={attempts}
        activity={activity ?? []}
        managedByStack={app.managed_by_stack}
      />
      <DeployAttemptsList
        appName={name}
        attempts={attempts}
        conditions={conditions}
        onDeploy={() => {
          setTab('existing-image')
        }}
      />
      <ScheduledDeploysList appName={name} />
      <BuildTrendCard appName={name} />
      <BuildLimitsCard appName={name} />
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-sm font-medium text-foreground">
              {t('deployHistory.settingsLink')}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('deployHistory.settingsLinkHint')}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={<Link to="/apps/$name/deploy-settings" params={{ name }} />}
          >
            <GearSixIcon className="size-3.5" data-icon="inline-start" />
            {t('deployHistory.settingsLinkCta')}
          </Button>
        </CardContent>
      </Card>
      <DeploySheet
        appName={name}
        tab={tab}
        onClose={() => {
          setTab(null)
        }}
      />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// DeployAttemptsList's own Card chrome so the skeleton doesn't jump when
// real rows swap in.
function DeploysSectionPending() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Deploy history</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4" aria-hidden="true">
        {Array.from({ length: 4 }, (_, i) => (
          <div key={i} className="flex items-start gap-3">
            <Skeleton className="mt-0.5 h-5 w-20 shrink-0 rounded-full" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-3 w-1/3" />
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
