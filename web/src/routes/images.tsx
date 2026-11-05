import { createFileRoute, redirect } from '@tanstack/react-router'
import {
  parseImageFilter,
  type ImageFilter,
} from '../components/images/imageFilters'

interface ImagesSearch {
  filter?: ImageFilter
}

// Old bookmarks and links keep working; the chip filter rides along.
export const Route = createFileRoute('/images')({
  validateSearch: (search: Record<string, unknown>): ImagesSearch => {
    const filter = parseImageFilter(search.filter)
    return filter === 'all' ? {} : { filter }
  },
  beforeLoad: ({ search }) => {
    redirect({ to: '/storage', search, replace: true, throw: true })
  },
})
