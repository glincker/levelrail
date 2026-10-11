import { useRef, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Badge } from '@/components/ui/badge'
import { FindingCard } from './FindingCard'
import {
  SEVERITY_BADGE,
  groupByResource,
  refKey,
  sortBySavings,
} from '../../lib/resourceAdvisor'
import { formatBytes } from '../../lib/format'
import type { AdvisorFinding } from '../../types/resourceAdvisor'

const VIRTUALIZE_OVER = 50

export type FindingGrouping = 'resource' | 'savings'

type Row =
  | { key: string; kind: 'header'; node: ReactNode }
  | { key: string; kind: 'finding'; finding: AdvisorFinding }

const LINK_CLASS =
  'text-sm font-semibold text-foreground underline-offset-4 hover:underline'

function ResourceLink({
  kind,
  name,
}: {
  kind: AdvisorFinding['resource']['kind']
  name: string
}) {
  if (kind === 'app') {
    return (
      <Link to="/apps/$name/resources" params={{ name }} className={LINK_CLASS}>
        {name}
      </Link>
    )
  }
  if (kind === 'database') {
    return (
      <Link
        to="/databases/$name/resources"
        params={{ name }}
        className={LINK_CLASS}
      >
        {name}
      </Link>
    )
  }
  return (
    <Link to="/nodes" className={LINK_CLASS}>
      {name}
    </Link>
  )
}

function GroupHeader({
  name,
  kind,
  severity,
  saving,
  count,
}: {
  name: string
  kind: AdvisorFinding['resource']['kind']
  severity: AdvisorFinding['severity']
  saving: number
  count: number
}) {
  const { t } = useTranslation('resourceAdvisor')
  return (
    <div className="flex flex-wrap items-center gap-2 pt-4 pb-1">
      <Badge variant="outline">{t(`kind.${kind}`)}</Badge>
      <ResourceLink kind={kind} name={name} />
      <Badge variant={SEVERITY_BADGE[severity]}>
        {t('summary.findings', { count })}
      </Badge>
      {saving > 0 ? (
        <span className="text-xs text-muted-foreground">
          {t('finding.saving', { amount: formatBytes(saving) })}
        </span>
      ) : null}
    </div>
  )
}

function buildRows(
  findings: AdvisorFinding[],
  grouping: FindingGrouping,
): Row[] {
  if (grouping === 'savings') {
    return sortBySavings(findings).map((f) => ({
      key: f.id,
      kind: 'finding',
      finding: f,
    }))
  }
  const rows: Row[] = []
  for (const g of groupByResource(findings)) {
    rows.push({
      key: `h:${refKey(g.ref)}`,
      kind: 'header',
      node: (
        <GroupHeader
          name={g.ref.name}
          kind={g.ref.kind}
          severity={g.severity}
          saving={g.saving}
          count={g.findings.length}
        />
      ),
    })
    for (const f of g.findings)
      rows.push({ key: f.id, kind: 'finding', finding: f })
  }
  return rows
}

function RowView({ row }: { row: Row }) {
  return row.kind === 'header' ? (
    row.node
  ) : (
    <FindingCard finding={row.finding} />
  )
}

function VirtualRows({ rows }: { rows: Row[] }) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 160,
    overscan: 6,
  })
  return (
    <div ref={parentRef} className="h-[70vh] overflow-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((v) => {
          const row = rows[v.index]
          if (!row) return null
          return (
            <div
              key={row.key}
              data-index={v.index}
              ref={virtualizer.measureElement}
              className="pb-3"
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${v.start}px)`,
              }}
            >
              <RowView row={row} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Lists findings grouped by resource or ordered by saving; long lists are
// virtualized so a large fleet never renders every card at once.
export function FindingList({
  findings,
  grouping,
}: {
  findings: AdvisorFinding[]
  grouping: FindingGrouping
}) {
  const rows = buildRows(findings, grouping)
  if (rows.length > VIRTUALIZE_OVER) return <VirtualRows rows={rows} />
  return (
    <div className="space-y-3">
      {rows.map((row) => (
        <RowView key={row.key} row={row} />
      ))}
    </div>
  )
}
