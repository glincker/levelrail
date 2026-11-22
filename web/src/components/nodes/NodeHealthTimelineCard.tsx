import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  useNodeTimeline,
  type NodeTimelineSegment,
  type TimelineWindow,
} from '../../queries/nodeInsight'

const WINDOWS: TimelineWindow[] = ['24h', '168h', '720h']
const STRIP_WIDTH = 1000

const STATE_FILL: Record<string, string> = {
  online: 'fill-tone-success',
  offline: 'fill-tone-danger',
  cordoned: 'fill-tone-warning',
  pending: 'fill-muted-foreground',
}

function Strip({
  segments,
  total,
  label,
}: {
  segments: NodeTimelineSegment[]
  total: number
  label: string
}) {
  const offsets = segments.map((_, i) =>
    segments
      .slice(0, i)
      .reduce((a, s) => a + (s.seconds / total) * STRIP_WIDTH, 0),
  )
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${STRIP_WIDTH} 24`}
      preserveAspectRatio="none"
      className="h-6 w-full overflow-hidden rounded"
    >
      {segments.map((s, i) => {
        const w = (s.seconds / total) * STRIP_WIDTH
        const rect = (
          <rect
            key={`${s.from}-${s.state}`}
            x={offsets[i]}
            y={0}
            width={w}
            height={24}
            className={STATE_FILL[s.state] ?? 'fill-muted'}
          >
            <title>{`${s.state}: ${new Date(s.from).toLocaleString()}`}</title>
          </rect>
        )
        return rect
      })}
    </svg>
  )
}

// Online and offline stretches of one node, with an uptime figure.
export function NodeHealthTimelineCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const [window, setWindow] = useState<TimelineWindow>('24h')
  const { data, isPending, isError } = useNodeTimeline(nodeId, window)
  const total = data ? data.segments.reduce((a, s) => a + s.seconds, 0) : 0

  return (
    <Card id="node-timeline">
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle>{t('timeline.title')}</CardTitle>
        <div
          className="flex gap-1"
          role="group"
          aria-label={t('timeline.window')}
        >
          {WINDOWS.map((w) => (
            <Button
              key={w}
              type="button"
              size="sm"
              variant={w === window ? 'secondary' : 'ghost'}
              aria-pressed={w === window}
              onClick={() => setWindow(w)}
            >
              {t(`timeline.windows.${w}`)}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {isPending ? (
          <div
            role="status"
            aria-label={t('timeline.loading')}
            className="h-6 animate-pulse rounded bg-muted motion-reduce:animate-none"
          />
        ) : isError ? (
          <p role="alert" className="text-sm text-muted-foreground">
            {t('timeline.unavailable')}
          </p>
        ) : (
          <>
            <Strip
              segments={data.segments}
              total={Math.max(total, 1)}
              label={t('timeline.stripLabel')}
            />
            <p className="text-sm text-muted-foreground">
              {data.uptime_percent === undefined
                ? t('timeline.noUptime')
                : t('timeline.summary', {
                    uptime: data.uptime_percent,
                    count: data.transitions,
                  })}
            </p>
          </>
        )}
      </CardContent>
    </Card>
  )
}
