// Query-key factory, fetchers, and hooks for the pipeline endpoints
// (internal/api/pipelines.go): definitions under /apps/{name}/pipelines,
// runs under /apps/{name}/pipeline-runs, plus POST /pipelines/validate.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  Pipeline,
  PipelineFiltersRequest,
  PipelineLogLine,
  PipelineRun,
  PipelineSaveRequest,
  PipelineStartRequest,
  PipelineStatus,
  PipelineSyncResult,
  PipelineSyncStatus,
  PipelineTriggerDecision,
  PipelineValidation,
} from '../types/pipelines'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const pipelineKeys = {
  all: (app: string) => [...appKeys.detail(app), 'pipelines'] as const,
  list: (app: string) => [...pipelineKeys.all(app), 'list'] as const,
  detail: (app: string, name: string) =>
    [...pipelineKeys.all(app), 'detail', name] as const,
  runs: (app: string, pipeline?: string) =>
    [...pipelineKeys.all(app), 'runs', pipeline ?? ''] as const,
  run: (app: string, id: string) =>
    [...pipelineKeys.all(app), 'run', id] as const,
  logs: (app: string, id: string, job: string, step?: number) =>
    [...pipelineKeys.all(app), 'logs', id, job, step ?? 'all'] as const,
  sync: (app: string) => [...pipelineKeys.all(app), 'sync'] as const,
  triggers: (app: string) => [...pipelineKeys.all(app), 'triggers'] as const,
}

const TERMINAL: PipelineStatus[] = ['succeeded', 'failed', 'cancelled']

export function isTerminalStatus(status: PipelineStatus): boolean {
  return TERMINAL.includes(status)
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}`
}

export async function requestJson<T>(
  url: string,
  what: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function pipelineListQueryOptions(app: string) {
  return queryOptions({
    queryKey: pipelineKeys.list(app),
    queryFn: () =>
      requestJson<Pipeline[]>(`${base(app)}/pipelines`, 'fetch pipelines'),
  })
}

export function usePipelines(app: string) {
  return useQuery(pipelineListQueryOptions(app))
}

export function usePipeline(app: string, name: string) {
  return useQuery({
    queryKey: pipelineKeys.detail(app, name),
    queryFn: () =>
      requestJson<Pipeline>(
        `${base(app)}/pipelines/${encodeURIComponent(name)}`,
        'fetch pipeline',
      ),
  })
}

export function usePipelineRuns(app: string, pipeline?: string) {
  const suffix = pipeline ? `&pipeline=${encodeURIComponent(pipeline)}` : ''
  return useQuery({
    queryKey: pipelineKeys.runs(app, pipeline),
    queryFn: () =>
      requestJson<PipelineRun[]>(
        `${base(app)}/pipeline-runs?limit=50${suffix}`,
        'fetch pipeline runs',
      ),
    refetchInterval: (query) =>
      query.state.data?.some((r) => !isTerminalStatus(r.status)) ? 3000 : false,
  })
}

export function usePipelineRun(app: string, id: string) {
  return useQuery({
    queryKey: pipelineKeys.run(app, id),
    queryFn: () =>
      requestJson<PipelineRun>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(id)}`,
        'fetch pipeline run',
      ),
    refetchInterval: (query) =>
      query.state.data && isTerminalStatus(query.state.data.status)
        ? false
        : 2000,
  })
}

export function usePipelineRunLogs(
  app: string,
  id: string,
  job: string,
  live = false,
  step?: number,
) {
  const stepParam = step === undefined ? '' : `&step=${step}`
  return useQuery({
    refetchInterval: live ? 2000 : false,
    queryKey: pipelineKeys.logs(app, id, job, step),
    queryFn: () =>
      requestJson<PipelineLogLine[]>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(id)}/logs?limit=5000&job=${encodeURIComponent(job)}${stepParam}`,
        'fetch pipeline logs',
      ),
  })
}

export function pipelineLogStreamUrl(
  app: string,
  id: string,
  job: string,
): string {
  return `${base(app)}/pipeline-runs/${encodeURIComponent(id)}/logs/stream?job=${encodeURIComponent(job)}`
}

export function useValidatePipeline() {
  return useMutation({
    mutationFn: (yaml: string) =>
      requestJson<PipelineValidation>(
        '/api/v1/pipelines/validate',
        'validate pipeline',
        jsonInit('POST', { yaml }),
      ),
  })
}

export function useApplyPipelineFilters() {
  return useMutation({
    mutationFn: async (req: PipelineFiltersRequest) => {
      const res = await requestJson<{ yaml: string }>(
        '/api/v1/pipelines/filters',
        'apply pipeline filters',
        jsonInit('POST', req),
      )
      return res.yaml
    },
  })
}

export function useSavePipeline(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      existing,
      req,
    }: {
      existing?: string
      req: PipelineSaveRequest
    }) =>
      existing
        ? requestJson<Pipeline>(
            `${base(app)}/pipelines/${encodeURIComponent(existing)}`,
            'save pipeline',
            jsonInit('PUT', req),
          )
        : requestJson<Pipeline>(
            `${base(app)}/pipelines`,
            'create pipeline',
            jsonInit('POST', req),
          ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function useDeletePipeline(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      requestJson<void>(
        `${base(app)}/pipelines/${encodeURIComponent(name)}`,
        'delete pipeline',
        { method: 'DELETE' },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function useStartPipelineRun(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, req }: { name: string; req: PipelineStartRequest }) =>
      requestJson<PipelineRun>(
        `${base(app)}/pipelines/${encodeURIComponent(name)}/runs`,
        'start pipeline run',
        jsonInit('POST', req),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function useCancelPipelineRun(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      requestJson<unknown>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(id)}/cancel`,
        'cancel pipeline run',
        { method: 'POST' },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function useRerunPipelineRun(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      requestJson<PipelineRun>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(id)}/rerun`,
        'rerun pipeline',
        { method: 'POST' },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function useDecidePipelineApproval(app: string, runId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      approvalId,
      decision,
      comment,
    }: {
      approvalId: number
      decision: 'approved' | 'rejected'
      comment?: string
    }) =>
      requestJson<unknown>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(runId)}/approvals/${approvalId}`,
        'decide approval',
        jsonInit('POST', { decision, comment: comment ?? '' }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.run(app, runId) })
    },
  })
}

export function usePipelineSync(app: string) {
  return useQuery({
    queryKey: pipelineKeys.sync(app),
    queryFn: () =>
      requestJson<PipelineSyncStatus>(
        `${base(app)}/pipeline-sync`,
        'fetch pipeline sync',
      ),
    retry: false,
  })
}

export function useSetPipelineRepoTruth(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (repoIsTruth: boolean) =>
      requestJson<PipelineSyncStatus>(
        `${base(app)}/pipeline-sync`,
        'save pipeline sync settings',
        jsonInit('PUT', { repo_is_truth: repoIsTruth }),
      ),
    onSuccess: (status) => {
      qc.setQueryData(pipelineKeys.sync(app), status)
    },
  })
}

export function useRunPipelineSync(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      requestJson<PipelineSyncResult>(
        `${base(app)}/pipeline-sync`,
        'sync pipelines',
        { method: 'POST' },
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}

export function usePipelineTriggers(app: string) {
  return useQuery({
    queryKey: pipelineKeys.triggers(app),
    queryFn: () =>
      requestJson<PipelineTriggerDecision[]>(
        `${base(app)}/pipeline-triggers?limit=30`,
        'fetch pipeline triggers',
      ),
  })
}

export function useDecidePipelineRunHold(app: string, runId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (decision: 'approved' | 'rejected') =>
      requestJson<unknown>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(runId)}/hold`,
        'decide held run',
        jsonInit('POST', { decision }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(app) })
    },
  })
}
