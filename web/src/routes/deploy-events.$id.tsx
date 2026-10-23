import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { DeployEventsPage } from '../components/deployEvents/DeployEventsPage'

interface DeployEventsSearch {
  with?: string
}

function validateSearch(search: Record<string, unknown>): DeployEventsSearch {
  const { with: other } = search
  return { with: typeof other === 'string' && other !== '' ? other : undefined }
}

export const Route = createFileRoute('/deploy-events/$id')({
  validateSearch,
  component: DeployEventsRoute,
})

function DeployEventsRoute() {
  const { id } = Route.useParams()
  const { with: other } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  return (
    <DeployEventsPage
      id={id}
      other={other ?? ''}
      onCompare={(next) => {
        void navigate({
          search: { with: next === '' ? undefined : next },
          replace: true,
        })
      }}
    />
  )
}
