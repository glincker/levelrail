import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

// Shapes of internal/api/node_patch_apply.go.
export interface PatchPackage {
  name: string
  current_version?: string
  new_version: string
  source?: string
  security: boolean
  kernel: boolean
  container_runtime: boolean
}

export interface PatchPreview {
  node_id: string
  manager: string
  include_all: boolean
  packages: PatchPackage[]
  held: PatchPackage[]
  other_available: number
  reboot_required_now: boolean
  reboot_likely: boolean
  preview_id: string
  expires_at: string
}

export type PatchRunStatus = 'running' | 'succeeded' | 'failed' | 'timed_out'

export interface PatchRun {
  id: string
  node_id: string
  actor: string
  manager: string
  include_all: boolean
  reboot_if_required: boolean
  status: PatchRunStatus
  started_at: string
  finished_at?: string
  packages: string[]
  held: string[]
  exit_code: number
  error?: string
  output_tail: string
  reboot_required: boolean
  reboot_scheduled: boolean
  reboot_note?: string
  patch_status_after?: { total: number; security: number }
}

// Poll interval for a running patch run, from env with a safe fallback.
const rawPoll = Number(import.meta.env.VITE_NODE_PATCH_POLL_MS)
export const PATCH_POLL_MS =
  Number.isFinite(rawPoll) && rawPoll > 0 ? rawPoll : 2000

export function isTerminalRun(status: PatchRunStatus): boolean {
  return status !== 'running'
}

const patchRunsKey = (id: string) =>
  [...nodeKeys.detail(id), 'patch-runs'] as const

async function post<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `node patch failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function previewNodePatch(
  id: string,
  body: { include_all: boolean; refresh: boolean },
): Promise<PatchPreview> {
  return post(`/api/v1/nodes/${encodeURIComponent(id)}/patch/preview`, body)
}

export function applyNodePatch(
  id: string,
  body: {
    preview_id: string
    confirm: true
    include_all: boolean
    reboot_if_required: boolean
  },
): Promise<PatchRun> {
  return post(`/api/v1/nodes/${encodeURIComponent(id)}/patch/apply`, body)
}

export async function fetchNodePatchRuns(id: string): Promise<PatchRun[]> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}/patch/runs`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch patch runs failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as PatchRun[] | { runs: PatchRun[] } | null
  if (Array.isArray(body)) return body
  return body?.runs ?? []
}

export function nodePatchRunsQueryOptions(id: string) {
  return queryOptions({
    queryKey: patchRunsKey(id),
    queryFn: () => fetchNodePatchRuns(id),
    refetchInterval: (query) =>
      query.state.data?.some((r) => r.status === 'running')
        ? PATCH_POLL_MS
        : false,
  })
}

export function useNodePatchRuns(id: string, enabled = true) {
  return useQuery({ ...nodePatchRunsQueryOptions(id), enabled, retry: false })
}

export function useNodePatchPreview(id: string) {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (body: { include_all: boolean; refresh: boolean }) =>
      previewNodePatch(id, body),
  })
}

export function useNodePatchApply(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (body: Parameters<typeof applyNodePatch>[1]) =>
      applyNodePatch(id, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: patchRunsKey(id) })
      void queryClient.invalidateQueries({ queryKey: nodeKeys.patchStatus(id) })
    },
  })
}
