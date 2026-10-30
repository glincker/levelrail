import { createFileRoute } from '@tanstack/react-router'
import { AssetsPage } from '../components/assets/AssetsPage'

interface AssetsSearch {
  bucket?: string
}

export const Route = createFileRoute('/assets')({
  validateSearch: (search: Record<string, unknown>): AssetsSearch =>
    typeof search.bucket === 'string' && search.bucket !== ''
      ? { bucket: search.bucket }
      : {},
  component: AssetsRoute,
})

function AssetsRoute() {
  const { bucket } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <AssetsPage
      bucketId={bucket}
      onSelect={(id) => {
        void navigate({ search: id ? { bucket: id } : {} })
      }}
    />
  )
}
