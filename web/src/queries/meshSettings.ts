import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { networkTopologyKeys } from './networkTopology'

export type MeshCheckStatus = 'pass' | 'warn' | 'fail'

export interface MeshCheck {
  id: string
  status: MeshCheckStatus
  detail: string
  hint?: string
}

// Matches internal/api's meshSettingsResource.
export interface MeshSettings {
  configured: boolean
  effective: boolean
  source: 'env' | 'settings' | 'profile' | 'default'
  env_var: string
  restart_required: boolean
  restart_hint?: string
  remote_nodes: number
  // host:port new agents dial; matches the agent certificate.
  agent_addr?: string
  checks: MeshCheck[]
  can_enable: boolean
  start_error?: string
  updated_by?: string
}

export const meshSettingsKeys = {
  all: ['mesh-settings'] as const,
}

async function fetchMeshSettings(): Promise<MeshSettings> {
  const res = await fetch('/api/v1/mesh/settings')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch mesh settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as MeshSettings
}

export function meshSettingsQueryOptions() {
  return queryOptions({
    queryKey: meshSettingsKeys.all,
    queryFn: fetchMeshSettings,
    staleTime: 10_000,
  })
}

export function useMeshSettings() {
  return useQuery(meshSettingsQueryOptions())
}

interface UpdateMeshSettingsInput {
  enabled: boolean
  force?: boolean
}

async function updateMeshSettings(
  input: UpdateMeshSettingsInput,
): Promise<MeshSettings> {
  const res = await fetch('/api/v1/mesh/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update mesh settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as MeshSettings
}

export function useUpdateMeshSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateMeshSettings,
    onSuccess: (data) => {
      queryClient.setQueryData(meshSettingsKeys.all, data)
      void queryClient.invalidateQueries({ queryKey: networkTopologyKeys.all })
    },
  })
}
