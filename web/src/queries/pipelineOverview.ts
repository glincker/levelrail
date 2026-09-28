// Query-key factory, fetchers, and hooks for the cross-app pipeline reads
// (GET /api/v1/pipeline-runs and /api/v1/pipelines/summary).

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  PipelineOverviewFilters,
  PipelineRunRow,
  PipelineRunRowsPage,
  PipelineSummary,
} from '../types/pipelineOverview'
import { jsonInit, pipelineKeys, requestJson } from './pipelines'

export const pipelineOverviewKeys = {
  all: ['pipeline-overview'] as const,
  summary: () => [...pipelineOverviewKeys.all, 'summary'] as const,
  runs: (filters: PipelineOverviewFilters, limit: number) =>
    [...pipelineOverviewKeys.all, 'runs', filters, limit] as const,
  infinite: (filters: PipelineOverviewFilters) =>
    [...pipelineOverviewKeys.all, 'infinite', filters] as const,
}

export const ACTIVE_POLL_MS = 3000
const IDLE_POLL_MS = 30000
const PAGE_SIZE = 50

function runsUrl(
  filters: PipelineOverviewFilters,
  limit: number,
  cursor?: string,
): string {
  const q = new URLSearchParams({ limit: String(limit) })
  const keys: (keyof PipelineOverviewFilters)[] = [
    'status',
    'app',
    'pipeline',
    'trigger',
  ]
  for (const key of keys) {
    const value = filters[key]
    if (value) {
      q.set(key, value)
    }
  }
  if (cursor) {
    q.set('cursor', cursor)
  }
  return `/api/v1/pipeline-runs?${q.toString()}`
}

export function isActiveRow(row: PipelineRunRow): boolean {
  return (
    row.status === 'running' ||
    row.status === 'queued' ||
    row.status === 'waiting_approval'
  )
}

export function usePipelineSummary() {
  return useQuery({
    queryKey: pipelineOverviewKeys.summary(),
    queryFn: () =>
      requestJson<PipelineSummary>(
        '/api/v1/pipelines/summary',
        'fetch pipeline summary',
      ),
    refetchInterval: (query) =>
      (query.state.data?.running ?? 0) > 0 ? ACTIVE_POLL_MS : IDLE_POLL_MS,
  })
}

export function usePipelineRunRows(filters: PipelineOverviewFilters) {
  return useInfiniteQuery({
    queryKey: pipelineOverviewKeys.infinite(filters),
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      requestJson<PipelineRunRowsPage>(
        runsUrl(filters, PAGE_SIZE, pageParam || undefined),
        'fetch pipeline runs',
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    refetchInterval: (query) =>
      query.state.data?.pages.some((p) => p.runs.some(isActiveRow))
        ? ACTIVE_POLL_MS
        : IDLE_POLL_MS,
  })
}

export function useAttentionRuns(
  filters: PipelineOverviewFilters,
  limit: number,
) {
  return useQuery({
    queryKey: pipelineOverviewKeys.runs(filters, limit),
    queryFn: () =>
      requestJson<PipelineRunRowsPage>(
        runsUrl(filters, limit),
        'fetch pipeline runs',
      ),
    refetchInterval: IDLE_POLL_MS,
  })
}

export interface RunDecision {
  row: PipelineRunRow
  decision: 'approved' | 'rejected'
}

export function useDecidePipelineRow() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ row, decision }: RunDecision) => {
      const run = `/api/v1/apps/${encodeURIComponent(row.app)}/pipeline-runs/${encodeURIComponent(row.id)}`
      if (row.hold_pending) {
        return requestJson<unknown>(
          `${run}/hold`,
          'decide held run',
          jsonInit('POST', { decision }),
        )
      }
      return requestJson<unknown>(
        `${run}/approvals/${row.approval_id ?? 0}`,
        'decide approval',
        jsonInit('POST', { decision, comment: '' }),
      )
    },
    onSuccess: (_data, { row }) => {
      void qc.invalidateQueries({ queryKey: pipelineOverviewKeys.all })
      void qc.invalidateQueries({ queryKey: pipelineKeys.all(row.app) })
    },
  })
}
