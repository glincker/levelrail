import { createFileRoute, redirect } from '@tanstack/react-router'
import { validateIntegrationsSearch } from '../../lib/integrationsSearch'

// Old home of the hub: bookmarks and attention links keep working with their params.
export const Route = createFileRoute('/settings/integrations')({
  validateSearch: validateIntegrationsSearch,
  beforeLoad: ({ search }) => {
    redirect({ to: '/integrations', search, replace: true, throw: true })
  },
})
