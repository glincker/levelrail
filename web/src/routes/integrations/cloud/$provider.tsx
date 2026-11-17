import { createFileRoute } from '@tanstack/react-router'
import { CloudProviderPage } from '../../../components/cloudProvider/CloudProviderPage'
import { pickStringSearch } from '../../../lib/urlDialog'
import { nodeProviderListQueryOptions } from '../../../queries/nodeProvision'

// ?builds=1 preselects the build role in the create wizard; connect and method
// drive the shared credential dialog.
export const Route = createFileRoute('/integrations/cloud/$provider')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { connect?: string; method?: string; builds?: string } =>
    pickStringSearch(search, ['connect', 'method', 'builds']),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(nodeProviderListQueryOptions()),
  component: CloudProviderRoute,
})

function CloudProviderRoute() {
  const { provider } = Route.useParams()
  const { builds } = Route.useSearch()
  return <CloudProviderPage providerId={provider} builds={builds === '1'} />
}
