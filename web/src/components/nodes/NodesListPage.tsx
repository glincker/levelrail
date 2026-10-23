import { useSuspenseInfiniteQuery } from '@tanstack/react-query'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PendingJoinTokensCard } from '../../components/nodes/PendingJoinTokensCard'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { nodePagesQueryOptions } from '../../queries/nodes'
import { useFleetResourceUsage } from '../../queries/fleetResourceUsage'
import { NODE_LIST_GRID, RowSkeleton } from '../../components/NodeRow'
import { NodesTable } from '../../components/NodesTable'
import { AddNodeWizard } from '../../components/AddNodeWizard'
import { BuildStrainSuggestion } from '../../components/BuildStrainSuggestion'
import { ZoneSpreadCard } from '../../components/nodes/ZoneSpreadCard'
import { ResilienceCard } from '../../components/nodes/ResilienceCard'
import { LazyNodeDomainsCard } from '../../components/nodes/LazyNodeOnboarding'
import { PageBanner } from '../PageBanner'
import { HelpLink } from '../HelpLink'
import { LazyNodesDiagram } from '../banners/LazyInfraDiagrams'
import type { NodeResource } from '../../types/nodeDetail'
import type { NodeResourceUsage } from '../../types/fleetResourceUsage'
import { LoadMore } from '../../components/ui/load-more'

// Column labels match NODE_LIST_GRID so the header never drifts from the rows.
function ListHeader() {
  const { t } = useTranslation('nodes')
  return (
    <div
      className={`${NODE_LIST_GRID} sticky top-0 z-10 hidden border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase lg:grid`}
    >
      <span aria-hidden="true" />
      <span>{t('list.name')}</span>
      <span>{t('list.address')}</span>
      <span>{t('list.usage')}</span>
      <span>{t('list.resources')}</span>
      <span>{t('list.agent')}</span>
      <span>{t('list.lastSeen')}</span>
      <span aria-hidden="true" />
    </div>
  )
}

const PAGE_WRAPPER =
  'mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)'

// Hero while this server is the only node, so the primary action is Add node.
export function NodeListView({
  nodes,
  total,
  usageByNodeId,
  counts,
  footer,
}: {
  nodes: NodeResource[]
  total: number
  usageByNodeId: ReadonlyMap<string, NodeResourceUsage>
  counts: ReadonlyMap<string, number>
  footer?: ReactNode
}) {
  const { t } = useTranslation('nodes')
  const solo = nodes.every((n) => n.is_local)
  const size = solo ? 'hero' : 'compact'
  return (
    <div className={PAGE_WRAPPER}>
      <PageBanner
        icon={<HardDrivesIcon />}
        titleKey="nodes:list.title"
        descriptionKey={
          solo ? 'nodes:banner.description' : 'nodes:list.description'
        }
        action={
          <>
            {nodes.length > 0 && !solo ? (
              <span className="text-sm text-muted-foreground">
                {t('list.count', { count: total })}
              </span>
            ) : null}
            <HelpLink path="/multi-node" label={t('list.help')} />
            <AddNodeWizard />
          </>
        }
        illustration={<LazyNodesDiagram size={size} />}
        size={size}
      />
      {solo ? (
        <p className="max-w-prose text-sm text-muted-foreground">
          {t('list.emptyDescription')}
        </p>
      ) : null}
      <PendingJoinTokensCard />
      <BuildStrainSuggestion nodes={nodes} />
      <ZoneSpreadCard />
      <LazyNodeDomainsCard />
      <ResilienceCard />
      {nodes.length > 0 ? (
        <div className="min-w-0 overflow-x-auto rounded-lg">
          <NodesTable
            nodes={nodes}
            usageByNodeId={usageByNodeId}
            counts={counts}
          />
        </div>
      ) : null}
      {footer}
    </div>
  )
}

export function NodeListPage() {
  const pages = useSuspenseInfiniteQuery(nodePagesQueryOptions())
  const nodes = useMemo(
    () => pages.data.pages.flatMap((p) => p.items),
    [pages.data],
  )
  const total = pages.data.pages.at(-1)?.total ?? nodes.length
  // Plain query: a 501 (telemetry not configured) is a real state, and the
  // list must render either way with the usage cells falling back to "-".
  const { data: usage } = useFleetResourceUsage()
  const usageByNodeId = new Map((usage?.nodes ?? []).map((n) => [n.node_id, n]))
  const counts = new Map(nodes.map((n) => [n.id, n.resource_count ?? 0]))
  return (
    <NodeListView
      nodes={nodes}
      total={total}
      usageByNodeId={usageByNodeId}
      counts={counts}
      footer={
        <LoadMore
          loaded={nodes.length}
          total={total}
          loading={pages.isFetchingNextPage}
          onLoadMore={() => {
            void pages.fetchNextPage()
          }}
        />
      }
    />
  )
}

export function NodeListPending() {
  return (
    <div className={PAGE_WRAPPER}>
      <PageBanner
        icon={<HardDrivesIcon />}
        titleKey="nodes:list.title"
        descriptionKey="nodes:list.description"
        size="compact"
      />
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}
