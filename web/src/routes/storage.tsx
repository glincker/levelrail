import { createFileRoute } from '@tanstack/react-router'
import {
  parseImageFilter,
  type ImageFilter,
} from '../components/images/imageFilters'
import { StorageHubPage } from '../components/storage/StorageHubPage'
import {
  parseStorageTab,
  type StorageTab,
} from '../components/storage/storageTabs'

interface StorageSearch {
  tab?: StorageTab
  filter?: ImageFilter
  review?: true
}

// Tab, chip and review flag live in the URL so a view can be shared or linked from a banner.
export const Route = createFileRoute('/storage')({
  validateSearch: (search: Record<string, unknown>): StorageSearch => {
    const tab = parseStorageTab(search.tab)
    const filter = parseImageFilter(search.filter)
    const review = search.review === '1' || search.review === true
    return {
      ...(tab === 'images' ? {} : { tab }),
      ...(filter === 'all' ? {} : { filter }),
      ...(review ? { review: true as const } : {}),
    }
  },
  component: StorageRoute,
})

function StorageRoute() {
  const { tab, filter, review } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StorageHubPage
      tab={tab ?? 'images'}
      filter={filter ?? 'all'}
      review={review === true}
      onTabChange={(next) => {
        void navigate({
          search: (prev) => ({
            ...prev,
            tab: next === 'images' ? undefined : next,
          }),
          replace: true,
        })
      }}
      onFilterChange={(next) => {
        void navigate({
          search: (prev) => ({
            ...prev,
            filter: next === 'all' ? undefined : next,
          }),
          replace: true,
        })
      }}
    />
  )
}
