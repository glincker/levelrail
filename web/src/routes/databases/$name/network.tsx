import { createFileRoute } from '@tanstack/react-router'
import { NetworkPanel } from '../../../components/databaseNetwork/NetworkPanel'

export const Route = createFileRoute('/databases/$name/network')({
  component: NetworkSection,
})

function NetworkSection() {
  const { name } = Route.useParams()
  return <NetworkPanel databaseName={name} />
}
