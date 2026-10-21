import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { deployLogsPath } from '../queries/deployments'
import { DeploymentsSectionTabs } from '../components/shell/SectionTabGroups'
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
  const goTo = useNavigate()
  return (
    <DeploymentsPage
      tabs={<DeploymentsSectionTabs />}
      search={parseDeploymentsSearch(search)}
      onViewApproval={() => {
        void goTo({ to: '/approvals' })
      }}
      onOpenDetails={(d) => {
        void goTo(deployLogsPath(d))
      }}
      onSearchChange={(next) => {
        void navigate({ search: next, replace: true })
      }}
    />
  )
}
