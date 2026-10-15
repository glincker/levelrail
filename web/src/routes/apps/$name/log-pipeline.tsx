import { createFileRoute } from '@tanstack/react-router'
import { LogPipelinePanel } from '../../../components/logPipeline/LogPipelinePanel'

export const Route = createFileRoute('/apps/$name/log-pipeline')({
  component: LogPipelineSection,
})

function LogPipelineSection() {
  const { name } = Route.useParams()
  return <LogPipelinePanel app={name} />
}
