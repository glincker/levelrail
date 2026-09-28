import { useMemo } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Timeline } from '@/components/kit'
import type { AppListEntry } from '../../types/appDetail'
import { flattenAttempts, pickSampleApps } from '../../lib/fleetStats'
import { toTimelineItems } from './activityItems'
import { useFleetDeploys } from '../../queries/fleetTraffic'

const ACTIVITY_APP_CAP = 6

export function RecentActivity({ apps }: { apps: AppListEntry[] }) {
  const navigate = useNavigate()
  const names = useMemo(() => pickSampleApps(apps, ACTIVITY_APP_CAP), [apps])
  const { byApp, isPending } = useFleetDeploys(names)
  const entries = flattenAttempts(byApp)
  return (
    <section aria-label="Recent activity" className="space-y-3">
      <h2 className="text-sm font-semibold text-foreground">Recent activity</h2>
      <Timeline
        loading={isPending}
        emptyLabel="No deploys yet. Push to a connected repo to see them here."
        items={toTimelineItems(entries, (app) => {
          void navigate({ to: '/apps/$name/deploys', params: { name: app } })
        })}
      />
    </section>
  )
}
