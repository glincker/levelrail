import { createFileRoute } from '@tanstack/react-router'
import { IntegrationsPage } from '../../components/integrations/IntegrationsPage'

// Read-only hub over the existing connection pages; ?view=catalog scrolls to the catalogue.
export const Route = createFileRoute('/settings/integrations')({
  validateSearch: (search: Record<string, unknown>): { view?: 'catalog' } =>
    search.view === 'catalog' ? { view: 'catalog' } : {},
  component: IntegrationsRoute,
})

function IntegrationsRoute() {
  const { view } = Route.useSearch()
  return <IntegrationsPage focusCatalog={view === 'catalog'} />
}
