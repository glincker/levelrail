// /api/v1/apps/{name}/log-pipeline and its preview endpoint.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { LogPipeline, LogRule, PreviewResult } from '../types/logPipeline'

export const logPipelineKeys = {
  detail: (app: string) => ['log-pipeline', app] as const,
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/log-pipeline`
}

async function ensureOk(res: Response, fallback: string): Promise<void> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
}

export function logPipelineQueryOptions(app: string) {
  return queryOptions({
    queryKey: logPipelineKeys.detail(app),
    queryFn: async () => {
      const res = await fetch(base(app))
      await ensureOk(res, `load pipeline failed: ${res.status}`)
      return (await res.json()) as LogPipeline
    },
  })
}

export function useLogPipeline(app: string) {
  return useQuery(logPipelineQueryOptions(app))
}

export function useSaveLogPipeline(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (rules: LogRule[]) => {
      const res = await fetch(base(app), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ rules }),
      })
      await ensureOk(res, `save pipeline failed: ${res.status}`)
      return (await res.json()) as LogPipeline
    },
    onSuccess: (data) => {
      qc.setQueryData(logPipelineKeys.detail(app), data)
    },
  })
}

export function useClearLogPipeline(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(base(app), { method: 'DELETE' })
      await ensureOk(res, `clear pipeline failed: ${res.status}`)
    },
    onSuccess: () => {
      qc.setQueryData(logPipelineKeys.detail(app), { rules: [] })
    },
  })
}

export interface PreviewInput {
  rules: LogRule[]
  lines: string[]
}

export function usePreviewLogPipeline(app: string) {
  return useMutation({
    mutationFn: async (input: PreviewInput) => {
      const res = await fetch(`${base(app)}/preview`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      await ensureOk(res, `preview failed: ${res.status}`)
      return (await res.json()) as PreviewResult
    },
  })
}
