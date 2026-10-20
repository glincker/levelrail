// Templates, failure fixes and run-over-run insights
// (internal/api/pipelines_ux.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { jsonInit, pipelineKeys, requestJson } from './pipelines'

export interface PipelineTemplate {
  id: string
  name: string
  language: string
  description: string
  tags: string[]
  yaml?: string
}

export type PipelineFixKind =
  | 'rerun'
  | 'add_retries'
  | 'raise_timeout'
  | 'raise_memory'
  | 'edit_pipeline'
  | 'set_secret'
  | 'free_disk'

export interface PipelineFix {
  code: string
  kind: PipelineFixKind
  job: string
  target?: string
  hint: string
  match: string
}

export interface PipelineFixResult {
  pipeline: string
  run?: string
}

export interface PipelineJobInsight {
  job: string
  runs: number
  failures: number
  failure_rate: number
  p50_seconds: number
  last_seconds: number
  flaky: boolean
  flaky_reason?: string
}

export interface PipelineJobDelta {
  job: string
  baseline_seconds: number
  current_seconds: number
  delta_seconds: number
  delta_percent: number
}

export interface PipelineInsights {
  window: number
  jobs: PipelineJobInsight[]
  diff_run_id?: string
  diff_base_run_id?: string
  diff: PipelineJobDelta[]
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}`
}

export function usePipelineTemplates(enabled = true) {
  return useQuery({
    enabled,
    queryKey: ['pipeline-templates'] as const,
    staleTime: 5 * 60_000,
    queryFn: () =>
      requestJson<PipelineTemplate[]>(
        '/api/v1/pipelines/templates',
        'fetch pipeline templates',
      ),
  })
}

export function usePipelineTemplate() {
  return useMutation({
    mutationFn: (id: string) =>
      requestJson<PipelineTemplate>(
        `/api/v1/pipelines/templates/${encodeURIComponent(id)}`,
        'fetch pipeline template',
      ),
  })
}

export function usePipelineRunFixes(
  app: string,
  runId: string,
  enabled: boolean,
) {
  return useQuery({
    enabled,
    queryKey: [...pipelineKeys.run(app, runId), 'fixes'] as const,
    queryFn: () =>
      requestJson<PipelineFix[]>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(runId)}/fixes`,
        'fetch pipeline fixes',
      ),
  })
}

export function useApplyPipelineFix(app: string, runId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: { kind: PipelineFixKind; job: string; rerun: boolean }) =>
      requestJson<PipelineFixResult>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(runId)}/fixes/apply`,
        'apply pipeline fix',
        jsonInit('POST', req),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function usePipelineInsights(app: string, name: string) {
  return useQuery({
    enabled: name !== '',
    queryKey: [...pipelineKeys.detail(app, name), 'insights'] as const,
    queryFn: () =>
      requestJson<PipelineInsights>(
        `${base(app)}/pipelines/${encodeURIComponent(name)}/insights`,
        'fetch pipeline insights',
      ),
  })
}
