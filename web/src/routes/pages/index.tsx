import { createFileRoute } from '@tanstack/react-router'
import { PagesList } from '../../components/pages/PagesList'

interface PagesSearch {
  new?: number
}

export const Route = createFileRoute('/pages/')({
  validateSearch: (search: Record<string, unknown>): PagesSearch => {
    const n = Number(search.new)
    return Number.isFinite(n) && n > 0 ? { new: n } : {}
  },
  component: PagesRoute,
})

function PagesRoute() {
  const { new: open } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PagesList
      initialOpen={open !== undefined}
      onClosed={() => void navigate({ search: {}, replace: true })}
    />
  )
}
