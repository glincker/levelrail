import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { TreeStructureIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PipelineAttentionStrip } from '../../components/PipelineAttentionStrip'
import { PipelineOverviewFilters } from '../../components/PipelineOverviewFilters'
import { PipelineOverviewTable } from '../../components/PipelineOverviewTable'
import { PipelineSummaryTiles } from '../../components/PipelineSummaryTiles'
import { PipelinesEmptyState } from '../../components/PipelinesEmptyState'
import { hasActiveFilters } from '../../lib/pipelineOverview'
import { usePipelineRunRows } from '../../queries/pipelineOverview'
import type { PipelineOverviewFilters as Filters } from '../../types/pipelineOverview'

export const Route = createFileRoute('/pipelines/')({
  component: PipelinesPage,
})

function PipelinesPage() {
  const [filters, setFilters] = useState<Filters>({})
  const query = usePipelineRunRows(filters)
  const rows = query.data?.pages.flatMap((p) => p.runs) ?? []
  const filtered = hasActiveFilters(filters)
  const showEmpty =
    !query.isLoading && !query.error && rows.length === 0 && !filtered

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <TreeStructureIcon className="size-4" aria-hidden="true" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Pipelines</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Recent CI/CD runs across every app you can read.
          </p>
        </div>
      </div>

      {showEmpty ? (
        <PipelinesEmptyState />
      ) : (
        <>
          <PipelineSummaryTiles />
          <PipelineAttentionStrip />
          <PipelineOverviewFilters filters={filters} onChange={setFilters} />
          {query.isLoading ? (
            <TableSkeleton columnCount={6} rowCount={6} />
          ) : query.error ? (
            <Alert variant="destructive">
              <AlertDescription>{query.error.message}</AlertDescription>
            </Alert>
          ) : rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No runs match these filters.
            </p>
          ) : (
            <PipelineOverviewTable
              rows={rows}
              hasMore={query.hasNextPage}
              loadingMore={query.isFetchingNextPage}
              onLoadMore={() => void query.fetchNextPage()}
            />
          )}
        </>
      )}
    </div>
  )
}
