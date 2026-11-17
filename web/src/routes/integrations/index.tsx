import { createFileRoute } from '@tanstack/react-router'
import { IntegrationsPage } from '../../components/integrations/IntegrationsPage'
import { validateIntegrationsSearch } from '../../lib/integrationsSearch'

// Read-only hub over the existing connection pages; ?view=catalog scrolls to the catalogue,
// ?connection=<id>&reauthorize=1 (attention links) lands on that git connection.
export const Route = createFileRoute('/integrations/')({
  validateSearch: validateIntegrationsSearch,
  component: IntegrationsRoute,
})

function IntegrationsRoute() {
  const { view, connection, reauthorize } = Route.useSearch()
  return (
    <IntegrationsPage
      focusCatalog={view === 'catalog'}
      reauthorizeConnection={reauthorize ? connection : undefined}
    />
  )
}
