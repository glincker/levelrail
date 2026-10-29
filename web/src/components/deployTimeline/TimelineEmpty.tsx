import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { RestoreGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { GhostPreview } from '../GhostPreview'
import type {
  Lane,
  TimelineDeploy,
  TimelineZoom,
} from '../../lib/deployTimeline'
import { TimelineLanes } from './TimelineLanes'

const SAMPLE_SHAPE = [
  { app: 'web', status: 'ready', at: 0.12, len: 0.05 },
  { app: 'web', status: 'failed', at: 0.4, len: 0.002 },
  { app: 'web', status: 'ready', at: 0.62, len: 0.06 },
  { app: 'api', status: 'ready', at: 0.25, len: 0.08 },
  { app: 'api', status: 'rolled_back', at: 0.7, len: 0.04 },
  { app: 'worker', status: 'ready', at: 0.5, len: 0.003 },
] as const

function sampleLanes(start: number, end: number): Lane[] {
  const span = end - start
  const byApp = new Map<string, TimelineDeploy[]>()
  SAMPLE_SHAPE.forEach((s, i) => {
    const at = start + span * s.at
    const done = at + span * s.len
    const d: TimelineDeploy = {
      id: `sample-${String(i)}`,
      app: s.app,
      status: s.status,
      trigger: '',
      environment: '',
      commit_sha: '',
      commit_message: '',
      author: '',
      strategy: '',
      project_id: '',
      node_id: '',
      started_at: new Date(at).toISOString(),
      finished_at: new Date(done).toISOString(),
      duration_ms: done - at,
      rollback_of: null,
      is_live: false,
    }
    byApp.set(s.app, [...(byApp.get(s.app) ?? []), d])
  })
  return [...byApp.entries()].map(([app, deploys]) => ({ app, deploys }))
}

/** Empty window: a faded sample chart behind the message, with a shortcut to the 7 day range. */
export function TimelineEmpty({
  zoom,
  windowStart,
  windowEnd,
  onShowWeek,
}: {
  zoom: TimelineZoom
  windowStart: number
  windowEnd: number
  onShowWeek: (() => void) | null
}) {
  const { t } = useTranslation('deployTimeline')
  const lanes = useMemo(
    () => sampleLanes(windowStart, windowEnd),
    [windowStart, windowEnd],
  )
  return (
    <GhostPreview
      sample={
        <TimelineLanes
          lanes={lanes}
          alerts={[]}
          zoom={zoom}
          windowStart={windowStart}
          windowEnd={windowEnd}
        />
      }
    >
      <EmptyState
        compact
        icon={<RestoreGlassIcon className="size-5.5" />}
        title={t('empty.title')}
        description={t('empty.body', { range: t(`zoom.${zoom}`) })}
        action={
          onShowWeek ? (
            <Button variant="outline" size="sm" onClick={onShowWeek}>
              {t('empty.action')}
            </Button>
          ) : undefined
        }
      />
    </GhostPreview>
  )
}
