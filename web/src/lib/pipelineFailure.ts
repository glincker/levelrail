import type { PipelineJob } from '../types/pipelines'

// firstFailure finds the first failed job and its first failed step.
export function firstFailure(
  jobs: PipelineJob[],
): { job: PipelineJob; stepName?: string; code?: number } | null {
  const job = jobs.find((j) => j.status === 'failed')
  if (!job) {
    return null
  }
  const step = job.steps.find((s) => s.status === 'failed')
  return { job, stepName: step?.name, code: step?.exit_code }
}
