import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's instanceResource (GET /api/v1/instance, public).
export interface InstanceInfo {
  instance_id: string
  name: string
  short_name: string
  version: string
  fingerprint: string
}

// Mirrors instanceIdentityResource (GET /api/v1/instance/identity).
export interface InstanceIdentity extends InstanceInfo {
  moved: boolean
  moved_at?: string
  previous_instance_id?: string
}

export const instanceKeys = {
  all: ['instance'] as const,
  info: () => [...instanceKeys.all, 'info'] as const,
  identity: () => [...instanceKeys.all, 'identity'] as const,
}

async function getJson<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useInstanceInfo() {
  return useQuery({
    queryKey: instanceKeys.info(),
    queryFn: () => getJson<InstanceInfo>('/api/v1/instance'),
    retry: false,
    staleTime: 60_000,
  })
}

export function useInstanceIdentity() {
  return useQuery({
    queryKey: instanceKeys.identity(),
    queryFn: () => getJson<InstanceIdentity>('/api/v1/instance/identity'),
    retry: false,
  })
}

export function useConfirmSameInstance() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (confirm: string): Promise<void> => {
      const res = await fetch('/api/v1/instance/confirm-same', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ confirm }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `confirm failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: instanceKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['tokens'] })
    },
  })
}
