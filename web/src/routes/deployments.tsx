import { createFileRoute } from '@tanstack/react-router'
import { DeploymentsPage } from '../components/deployments/DeploymentsPage'
import {
  parseDeploymentsSearch,
  toUrlSearch,
  type UrlSearch,
} from '../lib/deploymentFilters'

// Cross-app deploy list. Filters and the open drawer live in the URL so a view can be shared.
export const Route = createFileRoute('/deployments')({
  validateSearch: (search: Record<string, unknown>): UrlSearch => {
    const { d, ...filters } = parseDeploymentsSearch(search)
    return toUrlSearch(filters, d)
  },
  component: DeploymentsRoute,
})

function DeploymentsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <DeploymentsPage
      search={parseDeploymentsSearch(search)}
      onSearchChange={(next) => {
        void navigate({ search: next, replace: true })
      }}
    />
  )
}
