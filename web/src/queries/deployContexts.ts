import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { ContextConfig, EffectiveConfig } from '../types/deployContext'

export const deployContextKeys = {
  all: ['deploy-contexts'] as const,
  list: (app: string) => [...deployContextKeys.all, app, 'list'] as const,
  effective: (app: string, context: string, branch: string) =>
    [...deployContextKeys.all, app, 'effective', context, branch] as const,
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}`
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function useContextConfigs(app: string) {
  return useQuery({
    queryKey: deployContextKeys.list(app),
    queryFn: async (): Promise<ContextConfig[]> => {
      const res = await fetch(`${base(app)}/contexts`)
      if (!res.ok) throw await failure(res, 'fetch contexts')
      return (await res.json()) as ContextConfig[]
    },
  })
}

export function useEffectiveConfig(
  app: string,
  context: string,
  branch: string,
) {
  return useQuery({
    queryKey: deployContextKeys.effective(app, context, branch),
    queryFn: async (): Promise<EffectiveConfig> => {
      const q = new URLSearchParams({ context })
      if (branch) q.set('branch', branch)
      const res = await fetch(`${base(app)}/effective-config?${q.toString()}`)
      if (!res.ok) throw await failure(res, 'fetch effective config')
      return (await res.json()) as EffectiveConfig
    },
  })
}

export function useSaveContextConfig(app: string) {
  const queryClient = useQueryClient()
  return useMutation<ContextConfig, ApiError, ContextConfig>({
    mutationFn: async (cfg) => {
      const res = await fetch(`${base(app)}/contexts`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(cfg),
      })
      if (!res.ok) throw await failure(res, 'save context')
      return (await res.json()) as ContextConfig
    },
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: deployContextKeys.all }),
  })
}

export function useClearContextConfig(app: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (context) => {
      const res = await fetch(
        `${base(app)}/contexts?context=${encodeURIComponent(context)}`,
        { method: 'DELETE' },
      )
      if (!res.ok) throw await failure(res, 'clear context')
    },
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: deployContextKeys.all }),
  })
}
