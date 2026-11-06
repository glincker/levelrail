import { createFileRoute } from '@tanstack/react-router'
import type { ImageFilter } from '../components/images/imageFilters'
import type { StorageFilter } from '../components/storage/itemRows'
import { StorageHubPage } from '../components/storage/StorageHubPage'
import { normalizeStorageSearch } from '../components/storage/storageTabs'

// The images redirect still hands over image chips, so the type stays wide; values are normalised below.
interface StorageSearch {
  filter?: StorageFilter | ImageFilter
  view?: 'app'
  drawer?: 'cleanup'
}

// Filter, view and drawer live in the URL so a view can be shared or linked from a banner.
export const Route = createFileRoute('/storage')({
  validateSearch: (search: Record<string, unknown>): StorageSearch =>
    normalizeStorageSearch(search),
  component: StorageRoute,
})

function StorageRoute() {
  const { filter, view, drawer } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StorageHubPage
      filter={filter}
      view={view}
      drawerOpen={drawer === 'cleanup'}
      onFilterChange={(next) => {
        void navigate({
          search: (prev) => ({ ...prev, filter: next ?? undefined }),
          replace: true,
        })
      }}
      onViewChange={(next) => {
        void navigate({
          search: (prev) => ({
            ...prev,
            view: next === 'app' ? 'app' : undefined,
          }),
          replace: true,
        })
      }}
      onDrawerChange={(open) => {
        void navigate({
          search: (prev) => ({
            ...prev,
            drawer: open ? 'cleanup' : undefined,
          }),
          replace: true,
        })
      }}
    />
  )
}
