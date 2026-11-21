import { createFileRoute } from '@tanstack/react-router'
import { CloudAccountDetail } from '../../../components/cloudAccounts/CloudAccountDetail'

export const Route = createFileRoute('/integrations/cloud-accounts/$id')({
  component: CloudAccountRoute,
})

function CloudAccountRoute() {
  const { id } = Route.useParams()
  return <CloudAccountDetail id={id} />
}
