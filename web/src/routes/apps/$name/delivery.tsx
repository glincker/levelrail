import { createFileRoute } from '@tanstack/react-router'
import { DeliveryView } from '../../../components/delivery/DeliveryView'

// Pipeline and integrations timeline: pushes, runs, forge statuses, previews and deploys in one list.
export const Route = createFileRoute('/apps/$name/delivery')({
  component: DeliverySection,
})

function DeliverySection() {
  const { name } = Route.useParams()
  return <DeliveryView app={name} />
}
