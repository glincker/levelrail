import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseInfiniteQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import { appListQueryOptions } from '../../queries/apps'
import { AppRow, RowSkeleton } from '../../components/AppRow'

// This is the "App List" skeleton from frontend-plan.md section 4, kept
// close to the plan's example on purpose: typed loader primes the Query
// cache, the component only reads that cache via
// useSuspenseInfiniteQuery against the same key (no data fetching in the
// component body, per CLAUDE.md 7), and the list is virtualized
// unconditionally (frontend-plan.md section 3) rather than branching on
// row count.
export const Route = createFileRoute('/apps/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureInfiniteQueryData(appListQueryOptions()),
  component: AppListPage,
})

function AppListPage() {
  const { data, fetchNextPage, hasNextPage } = useSuspenseInfiniteQuery(
    appListQueryOptions(),
  )

  const rows = data.pages.flatMap((page) => page.items)
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: hasNextPage ? rows.length + 1 : rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 56,
    overscan: 8,
  })

  // Fetch the next page near scroll end. The virtualizer is the one
  // scroll-truth source here, no separate scroll listener.
  const items = virtualizer.getVirtualItems()
  const lastItem = items.at(-1)
  if (lastItem && lastItem.index >= rows.length - 1 && hasNextPage) {
    void fetchNextPage()
  }

  return (
    <div>
      <h1 className="mb-4 text-lg font-semibold text-neutral-900 dark:text-neutral-100">
        Apps
      </h1>
      {rows.length === 0 && !hasNextPage ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          No apps yet.
        </p>
      ) : (
        <div
          ref={parentRef}
          className="h-[70vh] overflow-auto rounded-lg border border-neutral-200 dark:border-neutral-800"
        >
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: 'relative',
            }}
          >
            {items.map((virtualRow) => {
              const app = rows[virtualRow.index]
              return (
                <div
                  key={virtualRow.key}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  {app ? <AppRow app={app} /> : <RowSkeleton />}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
