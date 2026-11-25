import { createFileRoute } from '@tanstack/react-router'
import { IntegrationsPage } from '../../../components/databases/home/IntegrationsPage'

export const Route = createFileRoute('/databases/$name/integrations')({
  component: IntegrationsSection,
})

function IntegrationsSection() {
  const { name } = Route.useParams()
  return <IntegrationsPage name={name} />
}
