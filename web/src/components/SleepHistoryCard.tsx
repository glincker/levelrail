import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { MoonGlassIcon, SunGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from './kit/RelativeTime'
import { useAppSleepActions } from '../queries/appSleep'
import { useSleepHistory, type SleepEvent } from '../queries/sleepHistory'
import { buildSleepSegments, formatSleepDuration } from '../lib/sleepTimeline'

const WINDOW_MS = 7 * 24 * 3600 * 1000
const EVENT_ROWS = 20

function SleepTimelineBar({
  events,
  currentSince,
  nowMs,
  label,
}: {
  events: SleepEvent[]
  nowMs: number
  currentSince: string | undefined
  label: string
}) {
  const segments = useMemo(
    () =>
      buildSleepSegments(
        events,
        nowMs,
        WINDOW_MS,
        currentSince ? Date.parse(currentSince) : null,
      ),
    [events, currentSince, nowMs],
  )
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 100 4"
      preserveAspectRatio="none"
      className="h-3 w-full overflow-hidden rounded-full bg-muted"
    >
      {segments.map((s) => (
        <rect
          key={`${s.startPct}-${s.asleep}`}
          x={s.startPct}
          y={0}
          width={s.widthPct}
          height={4}
          className={s.asleep ? 'fill-primary' : 'fill-success/60'}
        />
      ))}
    </svg>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm font-medium text-foreground">{value}</dd>
    </div>
  )
}

/** SleepHistoryCard shows when an app slept and woke, why, and how long wakes took. */
export function SleepHistoryCard({
  appName,
  hideWhenOff = false,
}: {
  appName: string
  hideWhenOff?: boolean
}) {
  const { t } = useTranslation('sleepHistory')
  const query = useSleepHistory(appName)
  const { setIdle } = useAppSleepActions(appName)
  const history = query.data

  if (query.isPending) {
    return (
      <Card>
        <CardContent
          className="py-6 text-sm text-muted-foreground"
          role="status"
        >
          {t('loading')}
        </CardContent>
      </Card>
    )
  }
  if (query.isError || !history) {
    return (
      <Card>
        <CardContent className="flex items-center justify-between gap-3 py-4">
          <p className="text-sm text-muted-foreground">{t('loadError')}</p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
          >
            {t('retry')}
          </Button>
        </CardContent>
      </Card>
    )
  }
  const noHistory = history.events.length === 0
  if (!history.enabled && noHistory) {
    if (hideWhenOff) return null
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <MoonGlassIcon className="size-4.5 text-muted-foreground" />
            {t('title')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm font-medium text-foreground">
            {t('disabled.title')}
          </p>
          <p className="text-sm text-muted-foreground">{t('disabled.body')}</p>
          <Button
            size="sm"
            disabled={setIdle.isPending}
            onClick={() =>
              setIdle.mutate(
                { idle_minutes: history.idle_limits.default_minutes },
                {
                  onSuccess: () => void query.refetch(),
                  onError: (error) =>
                    toast.add({
                      title: t('disabled.toastError'),
                      description: error.message,
                      type: 'error',
                    }),
                },
              )
            }
          >
            {t('disabled.action')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  const { stats } = history
  const none = t('stats.none')
  const ms = (v?: number) =>
    v === undefined ? none : formatSleepDuration(v / 1000)
  const rel = (v?: string) => (v ? <RelativeTime at={v} /> : none)
  const rows = history.events.slice(0, EVENT_ROWS)

  const columns: ResponsiveColumn<SleepEvent>[] = [
    {
      id: 'when',
      header: t('events.columns.when'),
      primary: true,
      cell: (e) => <RelativeTime at={e.at} />,
    },
    {
      id: 'event',
      header: t('events.columns.event'),
      cell: (e) => (
        <span className="flex items-center gap-1.5">
          {e.kind === 'woke' ? (
            <SunGlassIcon className="size-4" aria-hidden="true" />
          ) : (
            <MoonGlassIcon className="size-4" aria-hidden="true" />
          )}
          {t(`events.kind.${e.kind}`)}
        </span>
      ),
    },
    {
      id: 'reason',
      header: t('events.columns.reason'),
      priority: 2,
      cell: (e) =>
        e.reason
          ? t(`events.reason.${e.reason as 'manual'}`, {
              defaultValue: e.reason,
            })
          : none,
    },
    {
      id: 'measure',
      header: t('events.columns.measure'),
      priority: 2,
      cell: (e) => {
        const parts: string[] = []
        if (e.duration_seconds !== undefined)
          parts.push(
            t('events.sleptFor', {
              duration: formatSleepDuration(e.duration_seconds),
            }),
          )
        if (e.wake_latency_ms !== undefined)
          parts.push(
            t('events.readyIn', {
              duration: formatSleepDuration(e.wake_latency_ms / 1000),
            }),
          )
        return parts.length > 0 ? parts.join(', ') : none
      },
    },
    {
      id: 'detail',
      header: t('events.columns.detail'),
      priority: 3,
      cell: (e) => {
        const parts: string[] = []
        if (e.trigger_path) parts.push(e.trigger_path)
        if (e.trigger_source)
          parts.push(t('events.fromSource', { source: e.trigger_source }))
        if (e.idle_minutes)
          parts.push(t('events.idleAfter', { minutes: e.idle_minutes }))
        if (e.detail) parts.push(e.detail)
        if (e.actor && e.actor !== 'system')
          parts.push(t('events.byActor', { actor: e.actor }))
        return parts.length > 0 ? parts.join(', ') : none
      },
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <MoonGlassIcon className="size-4.5 text-muted-foreground" />
          {t('title')}
          <span className="text-xs font-normal text-muted-foreground">
            {stats.currently_asleep_since ? (
              <>
                {t('asleepNow', { since: '' })}
                <RelativeTime at={stats.currently_asleep_since} live />
              </>
            ) : (
              t('awakeNow')
            )}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <SleepTimelineBar
            events={history.events}
            currentSince={stats.currently_asleep_since}
            nowMs={query.dataUpdatedAt}
            label={t('timelineLabel')}
          />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>{t('timelineStart')}</span>
            <span className="flex items-center gap-3">
              <span className="flex items-center gap-1">
                <span className="size-2 rounded-full bg-primary" />
                {t('legend.asleep')}
              </span>
              <span className="flex items-center gap-1">
                <span className="size-2 rounded-full bg-success/60" />
                {t('legend.awake')}
              </span>
            </span>
            <span>{t('timelineEnd')}</span>
          </div>
        </div>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat
            label={t('stats.asleep7d')}
            value={formatSleepDuration(stats.asleep_seconds_7d)}
          />
          <Stat
            label={t('stats.asleep30d')}
            value={formatSleepDuration(stats.asleep_seconds_30d)}
          />
          <Stat
            label={t('stats.wakes7d')}
            value={String(stats.wake_count_7d)}
          />
          <Stat
            label={t('stats.avgLatency')}
            value={ms(stats.avg_wake_latency_ms)}
          />
          <Stat
            label={t('stats.p95Latency')}
            value={ms(stats.p95_wake_latency_ms)}
          />
          <Stat
            label={t('stats.memorySaved')}
            value={
              stats.memory_gib_hours_saved_30d === undefined
                ? none
                : t('stats.memorySavedValue', {
                    value: stats.memory_gib_hours_saved_30d.toFixed(1),
                  })
            }
          />
          <div className="min-w-0">
            <dt className="text-xs text-muted-foreground">
              {t('stats.lastSlept')}
            </dt>
            <dd className="text-sm font-medium text-foreground">
              {rel(stats.last_slept_at)}
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-xs text-muted-foreground">
              {t('stats.lastWoke')}
            </dt>
            <dd className="text-sm font-medium text-foreground">
              {rel(stats.last_woke_at)}
            </dd>
          </div>
        </dl>
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-foreground">
            {t('events.title')}
          </h3>
          {rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('events.empty')}</p>
          ) : (
            <ResponsiveTable
              rows={rows}
              columns={columns}
              getRowId={(e) => String(e.id)}
              ariaLabel={t('events.title')}
            />
          )}
        </div>
      </CardContent>
    </Card>
  )
}
