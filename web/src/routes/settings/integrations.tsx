import { createFileRoute } from '@tanstack/react-router'
import { IntegrationsPage } from '../../components/integrations/IntegrationsPage'

interface IntegrationsSearch {
  view?: 'catalog'
  connection?: string
  reauthorize?: boolean
}

// Read-only hub over the existing connection pages; ?view=catalog scrolls to the catalogue,
// ?connection=<id>&reauthorize=1 (attention links) lands on that git connection.
export const Route = createFileRoute('/settings/integrations')({
  validateSearch: (search: Record<string, unknown>): IntegrationsSearch => {
    const out: IntegrationsSearch = {}
    if (search.view === 'catalog') out.view = 'catalog'
    if (typeof search.connection === 'string' && search.connection !== '') {
      out.connection = search.connection
    }
    if (
      search.reauthorize === true ||
      search.reauthorize === 1 ||
      search.reauthorize === '1'
    ) {
      out.reauthorize = true
    }
    return out
  },
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
