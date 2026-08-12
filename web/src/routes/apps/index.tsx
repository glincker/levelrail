import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import { appListQueryOptions } from '../../queries/apps'
import { AppRow } from '../../components/AppRow'

// Typed loader primes the Query cache, the component only reads that
// cache via useSuspenseQuery against the same key (no data fetching in
// the component body, per CLAUDE.md 7). The list is virtualized
// unconditionally (every list that can exceed 50 items must be, per the
// same section) rather than branching on row count, even though GET
// /api/v1/apps returns everything in one response with no server-side
// pagination to page through.
export const Route = createFileRoute('/apps/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(appListQueryOptions()),
  component: AppListPage,
})

function AppListPage() {
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: apps.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 56,
    overscan: 8,
  })

  return (
    <div>
      <h1 className="mb-4 text-lg font-semibold text-neutral-900 dark:text-neutral-100">
        Apps
      </h1>
      {apps.length === 0 ? (
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
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const app = apps[virtualRow.index]
              if (!app) {
                return null
              }
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
                  <AppRow app={app} />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
