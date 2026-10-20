import { createFileRoute } from '@tanstack/react-router'
import { PipelineEditor } from '../../../../components/PipelineEditor'

export const Route = createFileRoute('/apps/$name/pipelines/$pipeline')({
  component: EditPipeline,
})

function EditPipeline() {
  const { name, pipeline } = Route.useParams()
  return <PipelineEditor appName={name} pipelineName={pipeline} />
}
