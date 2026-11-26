import { createFileRoute } from '@tanstack/react-router'
import { NetworkPanel } from '../../../components/databaseNetwork/NetworkPanel'

export const Route = createFileRoute('/databases/$name/networking')({
  component: NetworkingSection,
})

function NetworkingSection() {
  const { name } = Route.useParams()
  return <NetworkPanel databaseName={name} />
}
