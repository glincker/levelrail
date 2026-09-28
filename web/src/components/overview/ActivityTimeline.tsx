import { Link, useNavigate } from '@tanstack/react-router'
import { Timeline } from '@/components/kit'
import { useAppTimeline } from '../../queries/appTimeline'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { fallbackTimeline } from './timelineAdapter'
import { TIMELINE_MAX_ITEMS, toTimelineItems } from './timelineItems'

export function ActivityTimeline({
  app,
  attempts,
  conditions,
}: {
  app: AppDetail
  attempts: DeployAttempt[]
  conditions: ReconcileCondition[]
}) {
  const navigate = useNavigate()
  const timeline = useAppTimeline(app.name)
  const entries =
    timeline.data ??
    fallbackTimeline(attempts, app, conditions, TIMELINE_MAX_ITEMS)
  const items = toTimelineItems(entries, (deployId) => {
    void navigate({
      to: '/apps/$name/deploys/$deployId/logs',
      params: { name: app.name, deployId },
    })
  })

  return (
    <section aria-label="Activity" className="space-y-2">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-foreground">Activity</h2>
        <Link
          to="/apps/$name/deploys"
          params={{ name: app.name }}
          className="text-xs text-primary underline-offset-2 hover:underline"
        >
          View all activity
        </Link>
      </div>
      <Timeline
        items={items}
        loading={timeline.isPending}
        emptyLabel="Nothing yet. Your first deploy will show up here."
      />
    </section>
  )
}
