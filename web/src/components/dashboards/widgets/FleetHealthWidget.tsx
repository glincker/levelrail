import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { allocateWidths, widthClass } from '@/components/dashboard/widthClass'
import { TONE, type Tone } from '@/components/kit/tone'
import { appsSummaryQueryOptions } from '@/queries/appPages'
import { useNodeListOptional } from '@/queries/nodes'
import { nodeCounts } from '@/lib/fleetStats'
import { BigStat, Muted, WidgetSkeleton } from './parts'

const SEGMENTS = [
  { key: 'running', tone: 'success' },
  { key: 'failing', tone: 'danger' },
  { key: 'stopped', tone: 'neutral' },
] as const satisfies readonly { key: string; tone: Tone }[]

/** Apps by state as a segmented bar with numbers, plus nodes online. */
export function FleetHealthWidget() {
  const { t } = useTranslation('dashboards')
  const apps = useQuery({ ...appsSummaryQueryOptions(), retry: false })
  const nodes = useNodeListOptional()
  if (apps.isLoading) return <WidgetSkeleton />
  if (apps.error || !apps.data) return <Muted>{t('fleet.loadFailed')}</Muted>
  const s = apps.data
  if (s.total === 0) return <Muted>{t('fleet.health.empty')}</Muted>
  const values = SEGMENTS.map((x) => s[x.key])
  const widths = allocateWidths(values, s.total)
  const n = nodeCounts(nodes.data ?? [])
  return (
    <div className="space-y-3">
      <div
        role="img"
        aria-label={t('fleet.health.barLabel', {
          running: s.running,
          failing: s.failing,
          stopped: s.stopped,
        })}
        className="flex h-2 w-full overflow-hidden rounded-full bg-muted"
      >
        {SEGMENTS.map((x, i) => (
          <div
            key={x.key}
            className={cn(
              'h-full',
              TONE[x.tone].solid,
              widthClass(widths[i] ?? 0),
            )}
          />
        ))}
      </div>
      <div className="flex flex-wrap gap-x-6 gap-y-2">
        {SEGMENTS.map((x) => (
          <BigStat
            key={x.key}
            value={s[x.key]}
            label={t(`fleet.health.${x.key}`)}
            tone={x.tone === 'neutral' ? undefined : x.tone}
          />
        ))}
        <Link
          to="/nodes"
          className="rounded-sm hover:underline focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
        >
          <BigStat
            value={`${n.online} / ${n.total}`}
            label={t('fleet.health.nodesOnline')}
            tone={n.online < n.total ? 'warning' : undefined}
          />
        </Link>
      </div>
    </div>
  )
}
