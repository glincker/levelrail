import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { tokenKeys } from './tokens'

export const aiControlKeys = {
  all: ['ai-control'] as const,
}

export type AiControlMode = 'off' | 'observe' | 'operate' | 'admin'

// Matches internal/api's aiControlResource.
export interface AiControlSettings {
  mode: AiControlMode
  allowed_env_kinds: string[]
  env_kinds: string[]
  admin_available: boolean
  agent_token_count: number
  updated_at: string
  updated_by: string
}

export interface UpdateAiControlRequest {
  mode: AiControlMode
  allowed_env_kinds: string[]
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchAiControl(): Promise<AiControlSettings> {
  const res = await fetch('/api/v1/settings/ai-control')
  if (!res.ok) throw await failure(res, 'fetch AI control')
  return (await res.json()) as AiControlSettings
}

export function aiControlQueryOptions() {
  return queryOptions({
    queryKey: aiControlKeys.all,
    queryFn: fetchAiControl,
    staleTime: 10_000,
  })
}

export function useAiControl() {
  return useSuspenseQuery(aiControlQueryOptions())
}

export async function updateAiControl(
  req: UpdateAiControlRequest,
): Promise<AiControlSettings> {
  const res = await fetch('/api/v1/settings/ai-control', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) throw await failure(res, 'update AI control')
  return (await res.json()) as AiControlSettings
}

export function useUpdateAiControl() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateAiControl,
    onSuccess: (data) => {
      queryClient.setQueryData(aiControlKeys.all, data)
    },
  })
}

export async function revokeAgentTokens(): Promise<{ revoked: number }> {
  const res = await fetch('/api/v1/settings/ai-control/revoke-agent-tokens', {
    method: 'POST',
  })
  if (!res.ok) throw await failure(res, 'revoke agent tokens')
  return (await res.json()) as { revoked: number }
}

export function useRevokeAgentTokens() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: revokeAgentTokens,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: aiControlKeys.all })
      void queryClient.invalidateQueries({ queryKey: tokenKeys.list() })
    },
  })
}
