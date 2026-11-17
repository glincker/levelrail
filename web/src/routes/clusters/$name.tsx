import { createFileRoute } from '@tanstack/react-router'
import { ClusterDeploymentsPage } from '../../components/clusters/ClusterDeploymentsPage'

export const Route = createFileRoute('/clusters/$name')({
  component: ClusterRoute,
})

function ClusterRoute() {
  const { name } = Route.useParams()
  return <ClusterDeploymentsPage name={name} />
}
