import { createFileRoute } from '@tanstack/react-router'
import { ImagesPage } from '../components/images/ImagesPage'
import {
  parseImageFilter,
  type ImageFilter,
} from '../components/images/imageFilters'

interface ImagesSearch {
  filter?: ImageFilter
}

// The chip lives in the URL so a filtered view can be shared or reloaded.
export const Route = createFileRoute('/images')({
  validateSearch: (search: Record<string, unknown>): ImagesSearch => {
    const filter = parseImageFilter(search.filter)
    return filter === 'all' ? {} : { filter }
  },
  component: ImagesRoute,
})

function ImagesRoute() {
  const { filter } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ImagesPage
      filter={filter ?? 'all'}
      onFilterChange={(next) => {
        void navigate({
          search: next === 'all' ? {} : { filter: next },
          replace: true,
        })
      }}
    />
  )
}
