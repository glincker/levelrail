import { createFileRoute } from '@tanstack/react-router'
import { DomainDetailPage } from '../../../components/domains/DomainDetailPage'

// Trailing underscore keeps this page out of the domains list layout.
export const Route = createFileRoute('/apps/$name/domains_/$domain')({
  component: DomainDetailRoute,
})

function DomainDetailRoute() {
  const { name, domain } = Route.useParams()
  return <DomainDetailPage appName={name} domain={domain} />
}
