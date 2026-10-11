import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Button } from '@/components/ui/button'
import {
  formatDeltaPct,
  formatMoney,
  groupName,
  sparklinePoints,
} from '../../lib/usageFormat'
import type { UsageDimension, UsageGroup, UsageReport } from '../../types/usage'

const DIMENSIONS: UsageDimension[] = [
  'project',
  'environment',
  'team',
  'resource',
]
const VIRTUALIZE_ABOVE = 50
const ROW_HEIGHT = 44
const GRID =
  'grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 sm:grid-cols-[minmax(0,1fr)_6rem_6rem_5rem_6rem]'

function rowsFor(report: UsageReport, dim: UsageDimension): UsageGroup[] {
  switch (dim) {
    case 'environment':
      return report.environments
    case 'team':
      return report.teams
    case 'resource':
      return report.resources
    default:
      return report.projects
  }
}

function Sparkline({ daily }: { daily: number[] }) {
  const points = sparklinePoints(daily)
  if (!points) return <span aria-hidden="true" />
  return (
    <svg
      viewBox="0 0 100 24"
      preserveAspectRatio="none"
      className="h-6 w-24 text-muted-foreground"
      aria-hidden="true"
    >
      <polyline
        points={points}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}

function Row({
  report,
  dim,
  g,
}: {
  report: UsageReport
  dim: UsageDimension
  g: UsageGroup
}) {
  const { t } = useTranslation('usage')
  const name = groupName(report, dim, g, t('table.unassigned'))
  return (
    <div
      role="row"
      className={`${GRID} border-b border-border px-3 py-2 text-sm`}
    >
      <span role="cell" className="truncate font-medium text-foreground">
        {name}
        {dim === 'resource' && g.kind ? (
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            {g.kind}
          </span>
        ) : null}
      </span>
      <span role="cell" className="text-right tabular-nums">
        {formatMoney(g.total, report.currency)}
      </span>
      <span
        role="cell"
        className="hidden text-right tabular-nums text-muted-foreground sm:block"
      >
        {formatMoney(g.previous, report.currency)}
      </span>
      <span role="cell" className="text-right tabular-nums">
        {formatDeltaPct(g.delta_pct)}
      </span>
      <span role="cell" className="hidden sm:block">
        <Sparkline daily={g.daily} />
      </span>
    </div>
  )
}

function VirtualRows({
  report,
  dim,
  rows,
}: {
  report: UsageReport
  dim: UsageDimension
  rows: UsageGroup[]
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })
  return (
    <div ref={parentRef} className="max-h-[60vh] overflow-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((v) => {
          const g = rows[v.index]
          if (!g) return null
          return (
            <div
              key={g.key}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${v.start}px)`,
              }}
            >
              <Row report={report} dim={dim} g={g} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Ranked breakdown of the report by project, environment, team or resource.
export function UsageTable({ report }: { report: UsageReport }) {
  const { t } = useTranslation('usage')
  const [dim, setDim] = useState<UsageDimension>('project')
  const rows = rowsFor(report, dim)
  return (
    <section className="space-y-3">
      <div
        className="flex flex-wrap gap-1"
        role="group"
        aria-label={t('dimension.label')}
      >
        {DIMENSIONS.map((d) => (
          <Button
            key={d}
            size="sm"
            variant={dim === d ? 'secondary' : 'ghost'}
            aria-pressed={dim === d}
            onClick={() => setDim(d)}
          >
            {t(`dimension.${d}`)}
          </Button>
        ))}
      </div>
      <div
        role="table"
        aria-label={t('table.label')}
        aria-rowcount={rows.length + 1}
        className="rounded-lg border border-border"
      >
        <div
          role="row"
          className={`${GRID} border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground`}
        >
          <span role="columnheader">{t('table.name')}</span>
          <span role="columnheader" className="text-right">
            {t('table.cost')}
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            {t('table.previous')}
          </span>
          <span role="columnheader" className="text-right">
            {t('table.change')}
          </span>
          <span role="columnheader" className="hidden sm:block">
            {t('table.trend')}
          </span>
        </div>
        {rows.length === 0 ? (
          <p className="px-3 py-6 text-sm text-muted-foreground">
            {t('table.empty')}
          </p>
        ) : rows.length > VIRTUALIZE_ABOVE ? (
          <VirtualRows report={report} dim={dim} rows={rows} />
        ) : (
          rows.map((g) => <Row key={g.key} report={report} dim={dim} g={g} />)
        )}
      </div>
    </section>
  )
}
