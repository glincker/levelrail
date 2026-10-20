import { createFileRoute } from '@tanstack/react-router'
import { PipelineRunDetail } from '../../../../../components/PipelineRunDetail'

interface RunSearch {
  job?: string
  step?: number
}

// Plain function rather than zod so validateSearch stays out of the eagerly loaded bundle.
function validateRunSearch(search: Record<string, unknown>): RunSearch {
  const step = Number(search.step)
  return {
    ...(typeof search.job === 'string' && search.job !== ''
      ? { job: search.job }
      : {}),
    ...(search.step !== undefined && Number.isInteger(step) && step >= 0
      ? { step }
      : {}),
  }
}

export const Route = createFileRoute('/apps/$name/pipelines/runs/$runId')({
  validateSearch: validateRunSearch,
  component: PipelineRunPage,
})

function PipelineRunPage() {
  const { name, runId } = Route.useParams()
  const { job, step } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PipelineRunDetail
      app={name}
      runId={runId}
      job={job}
      step={step}
      onPick={(nextJob, nextStep) =>
        void navigate({
          search: {
            job: nextJob,
            ...(nextStep === undefined ? {} : { step: nextStep }),
          },
          replace: true,
        })
      }
    />
  )
}
