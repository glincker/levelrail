import { createFileRoute } from '@tanstack/react-router'
import { PipelineRunDetail } from '../../../../../components/PipelineRunDetail'

export const Route = createFileRoute('/apps/$name/pipelines/runs/$runId')({
  component: PipelineRunPage,
})

function PipelineRunPage() {
  const { name, runId } = Route.useParams()
  return <PipelineRunDetail app={name} runId={runId} />
}
