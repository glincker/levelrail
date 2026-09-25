import { createFileRoute } from '@tanstack/react-router'
import { PipelineEditor } from '../../../../components/PipelineEditor'

export const Route = createFileRoute('/apps/$name/pipelines/new')({
  component: NewPipeline,
})

function NewPipeline() {
  const { name } = Route.useParams()
  return <PipelineEditor appName={name} />
}
