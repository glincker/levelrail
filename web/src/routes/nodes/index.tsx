import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseInfiniteQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { PendingJoinTokensCard } from '../../components/nodes/PendingJoinTokensCard'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { nodePagesQueryOptions } from '../../queries/nodes'
import { useFleetResourceUsage } from '../../queries/fleetResourceUsage'
import { NODE_LIST_GRID, RowSkeleton } from '../../components/NodeRow'
import { NodesTable } from '../../components/NodesTable'
import { AddNodeWizard } from '../../components/AddNodeWizard'
import { BuildStrainSuggestion } from '../../components/BuildStrainSuggestion'
import { EmptyState } from '../../components/ui/empty-state'
import { ZoneSpreadCard } from '../../components/nodes/ZoneSpreadCard'
import { ResilienceCard } from '../../components/nodes/ResilienceCard'
import { PageTemplate } from '../../components/shell/PageTemplate'
import { LoadMore } from '../../components/ui/load-more'

// The loader primes the cache; the table virtualizes itself past 50 rows.
export const Route = createFileRoute('/nodes/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureInfiniteQueryData(nodePagesQueryOptions()),
  component: NodeListPage,
  pendingComponent: NodeListPending,
})

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

function NodeListPage() {
  const { t } = useTranslation('nodes')
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
    <PageTemplate
      title={t('list.title')}
      description={t('list.description')}
      helpPath="/multi-node"
      helpLabel={t('list.help')}
      actions={
        <>
          {nodes.length > 0 ? (
            <span className="text-sm text-muted-foreground">
              {t('list.count', { count: total })}
            </span>
          ) : null}
          <AddNodeWizard />
        </>
      }
    >
      <PendingJoinTokensCard />
      <BuildStrainSuggestion nodes={nodes} />
      <ZoneSpreadCard />
      <ResilienceCard />
      {nodes.length === 0 ? (
        <EmptyState
          icon={<HardDrivesIcon className="size-5" />}
          title={t('list.emptyTitle')}
          description={t('list.emptyDescription')}
          action={<AddNodeWizard />}
        />
      ) : (
        <NodesTable
          nodes={nodes}
          usageByNodeId={usageByNodeId}
          counts={counts}
        />
      )}
      <LoadMore
        loaded={nodes.length}
        total={total}
        loading={pages.isFetchingNextPage}
        onLoadMore={() => {
          void pages.fetchNextPage()
        }}
      />
    </PageTemplate>
  )
}

function NodeListPending() {
  const { t } = useTranslation('nodes')
  return (
    <PageTemplate title={t('list.title')}>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </PageTemplate>
  )
}
