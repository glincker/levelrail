import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DockerGuardMode = 'off' | 'audit' | 'enforce'

export interface DockerGuardRuleSummary {
  rule: string
  denied: number
  would_deny: number
  last_seen: string
  last_path: string
}

// Matches internal/api's dockerGuardResource.
export interface DockerGuardStatus {
  mode: DockerGuardMode
  source: 'env' | 'settings' | 'default'
  effective: DockerGuardMode
  running: boolean
  socket?: string
  upstream?: string
  restart_required: boolean
  config_error?: string
  start_error?: string
  audit_since?: string
  rules: string[]
  configured: boolean
  window_seconds: number
  window: DockerGuardRuleSummary[]
  would_deny_total: number
  denied_total: number
  ready_to_enforce: boolean
}

export const dockerGuardKeys = {
  all: ['docker-guard'] as const,
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

async function fetchDockerGuard(): Promise<DockerGuardStatus> {
  const res = await fetch('/api/v1/system/docker-guard')
  if (!res.ok) throw await failure(res, 'fetch docker guard')
  return (await res.json()) as DockerGuardStatus
}

export function dockerGuardQueryOptions() {
  return queryOptions({
    queryKey: dockerGuardKeys.all,
    queryFn: fetchDockerGuard,
    staleTime: 10_000,
  })
}

export function useDockerGuard() {
  return useQuery(dockerGuardQueryOptions())
}

async function updateDockerGuard(
  mode: DockerGuardMode,
): Promise<DockerGuardStatus> {
  const res = await fetch('/api/v1/system/docker-guard', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  })
  if (!res.ok) throw await failure(res, 'update docker guard')
  return (await res.json()) as DockerGuardStatus
}

export function useUpdateDockerGuard() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateDockerGuard,
    onSuccess: (data) => {
      queryClient.setQueryData(dockerGuardKeys.all, data)
    },
  })
}
