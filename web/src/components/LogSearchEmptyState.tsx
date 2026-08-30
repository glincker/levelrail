import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'
import { EmptyState } from './ui/empty-state'
import type { TimeRangeKey } from '../lib/timeRange'

const WIDEST_RANGE_KEY: TimeRangeKey = '7d'

// Shared zero-result state for LogSearchPanel and DatabaseLogSearchPanel:
// both search the same shape of data (a query plus a time-range preset)
// against a different endpoint, so the "why is this empty, and what do I
// do about it" copy and the Clear filters action are identical apart
// from which resource kind's name appears in the fallback sentence.
export interface LogSearchState {
  query: string
  rangeKey: TimeRangeKey
  setQuery: (value: string) => void
  setRangeKey: (value: TimeRangeKey) => void
}

export function LogSearchEmptyState({
  resourceKind,
  search,
}: {
  resourceKind: 'app' | 'database'
  search: LogSearchState
}) {
  const { query, rangeKey, setQuery, setRangeKey } = search
  const canBroaden = query.trim() !== '' || rangeKey !== WIDEST_RANGE_KEY

  function clearFilters() {
    setQuery('')
    setRangeKey(WIDEST_RANGE_KEY)
  }

  return (
    <EmptyState
      className="py-8"
      icon={<MagnifyingGlassIcon className="size-5" />}
      title="No log entries found"
      description={
        query
          ? `Nothing matched "${query}" in the selected time range.`
          : `No log output was captured for this ${resourceKind} in the selected time range.`
      }
      action={
        canBroaden ? (
          <Button size="sm" variant="outline" onClick={clearFilters}>
            Clear filters
          </Button>
        ) : undefined
      }
    />
  )
}
