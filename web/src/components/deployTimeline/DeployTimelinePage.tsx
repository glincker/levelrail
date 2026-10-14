import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ClockCounterClockwiseIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { StatusPill, SkeletonList } from '@/components/kit'
import { PageTemplate } from '../shell/PageTemplate'
import { useDeployTimeline } from '../../queries/deployTimeline'
import { useProjectListOptional } from '../../queries/projects'
import { useNodeListOptional } from '../../queries/nodes'
import {
  DEFAULT_TIMELINE_FILTERS,
  groupLanes,
  type TimelineDeploy,
  type TimelineFilters as Filters,
} from '../../lib/deployTimeline'
import { TimelineFilters } from './TimelineFilters'
import { TimelineHoverCard } from './TimelineHoverCard'
import { TimelineLanes } from './TimelineLanes'

const STREAM_TONE = {
  connecting: 'neutral',
  live: 'success',
  reconnecting: 'warning',
} as const

export function DeployTimelinePage() {
  const { t } = useTranslation('deployTimeline')
  const [filters, setFilters] = useState<Filters>(DEFAULT_TIMELINE_FILTERS)
  const [active, setActive] = useState<TimelineDeploy | null>(null)
  const { query, items, streamState, windowStart, windowEnd } =
    useDeployTimeline(filters)
  const projects = useProjectListOptional().data
  const nodes = useNodeListOptional().data

  const lanes = useMemo(() => groupLanes(items), [items])
  const appNames = useMemo(() => {
    const names = new Set(items.map((d) => d.app))
    if (filters.app) names.add(filters.app)
    return [...names].sort()
  }, [items, filters.app])

  return (
    <PageTemplate
      title={t('title')}
      description={t('subtitle')}
      width="wide"
      actions={
        <StatusPill
          size="sm"
          tone={STREAM_TONE[streamState]}
          label={t(`stream.${streamState}`)}
          live={streamState === 'live'}
        />
      }
    >
      <div className="flex flex-col gap-4">
        <TimelineFilters
          filters={filters}
          apps={appNames}
          projects={(projects ?? []).map((p) => ({ id: p.id, name: p.name }))}
          nodes={(nodes ?? []).map((n) => ({ id: n.id, name: n.name }))}
          onChange={setFilters}
        />
        {query.isPending ? (
          <SkeletonList rows={6} />
        ) : query.isError ? (
          <EmptyState
            icon={<WarningCircleIcon />}
            title={t('error.title')}
            description={query.error.message}
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void query.refetch()
                }}
              >
                {t('error.retry')}
              </Button>
            }
          />
        ) : lanes.length === 0 ? (
          <EmptyState
            icon={<ClockCounterClockwiseIcon />}
            title={t('empty.title')}
            description={t('empty.body', { range: t(`zoom.${filters.zoom}`) })}
            action={
              filters.zoom !== '7d' ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setFilters({ ...filters, zoom: '7d' })
                  }}
                >
                  {t('empty.action')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <>
            <TimelineHoverCard deploy={active} />
            <TimelineLanes
              lanes={lanes}
              zoom={filters.zoom}
              windowStart={windowStart}
              windowEnd={windowEnd}
              activeId={active?.id ?? null}
              onActive={setActive}
            />
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
              <span>
                {t('count', { deploys: items.length, apps: lanes.length })}
              </span>
              {query.hasNextPage ? (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={query.isFetchingNextPage}
                  onClick={() => {
                    void query.fetchNextPage()
                  }}
                >
                  {query.isFetchingNextPage
                    ? t('more.loading')
                    : t('more.label')}
                </Button>
              ) : null}
            </div>
          </>
        )}
      </div>
    </PageTemplate>
  )
}
