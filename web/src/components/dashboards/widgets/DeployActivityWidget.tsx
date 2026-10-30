import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit/RelativeTime'
import { SAMPLE_APP_CAP } from '@/components/dashboard/FleetTiles'
import { useMinuteNow } from '@/hooks/useMinuteNow'
import { flattenAttempts, pickSampleApps } from '@/lib/fleetStats'
import { useAppListOptional } from '@/queries/apps'
import { useFleetDeploys } from '@/queries/fleetTraffic'
import { Muted, WidgetSkeleton } from './parts'

const RECENT_ROWS = 3
const DAY_MS = 86_400_000

/** Deploy counts for the last day and week plus the latest three, from the busiest sampled apps. */
export function DeployActivityWidget() {
  const { t } = useTranslation('dashboards')
  const now = useMinuteNow()
  const apps = useAppListOptional()
  const names = useMemo(
    () => pickSampleApps(apps.data ?? [], SAMPLE_APP_CAP),
    [apps.data],
  )
  const deploys = useFleetDeploys(names)
  if (apps.isLoading || deploys.isPending) return <WidgetSkeleton />
  if (apps.error) return <Muted>{t('fleet.loadFailed')}</Muted>
  const done = flattenAttempts(deploys.byApp).filter(
    (e) => e.status === 'succeeded' || e.status === 'failed',
  )
  if (done.length === 0) return <Muted>{t('fleet.deploys.empty')}</Muted>
  const within = (ms: number) =>
    done.filter((e) => now - new Date(e.at).getTime() < ms)
  const day = within(DAY_MS)
  const week = within(7 * DAY_MS)
  const ok = (l: typeof done) =>
    l.filter((e) => e.status === 'succeeded').length
  return (
    <div className="space-y-2">
      <p className="text-sm tabular-nums">
        {t('fleet.deploys.summary', {
          day: day.length,
          week: week.length,
          ok: ok(week),
          failed: week.length - ok(week),
        })}
      </p>
      <ul className="space-y-1 text-sm">
        {done.slice(0, RECENT_ROWS).map((e) => {
          const good = e.status === 'succeeded'
          const Icon = good ? CircleCheckGlassIcon : CircleAlertGlassIcon
          return (
            <li key={e.id}>
              <Link
                to="/apps/$name/deploys"
                params={{ name: e.app }}
                className="flex items-center gap-2 rounded-sm hover:underline focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
              >
                <Icon
                  aria-hidden="true"
                  className={
                    good
                      ? 'size-4 text-tone-success'
                      : 'size-4 text-tone-danger'
                  }
                />
                <span className="min-w-0 flex-1 truncate">{e.app}</span>
                <span
                  className={good ? 'text-tone-success' : 'text-tone-danger'}
                >
                  {t(good ? 'fleet.deploys.succeeded' : 'fleet.deploys.failed')}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  <RelativeTime at={e.at} />
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
      {(apps.data?.length ?? 0) > SAMPLE_APP_CAP ? (
        <p className="text-xs text-muted-foreground">
          {t('fleet.deploys.sampled', { count: SAMPLE_APP_CAP })}
        </p>
      ) : null}
    </div>
  )
}
