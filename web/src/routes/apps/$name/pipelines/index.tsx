import { createFileRoute } from '@tanstack/react-router'
import { PipelinesPanel } from '../../../../components/PipelinesPanel'

export const Route = createFileRoute('/apps/$name/pipelines/')({
  component: PipelinesSection,
})

function PipelinesSection() {
  const { name } = Route.useParams()
  return <PipelinesPanel appName={name} />
}
