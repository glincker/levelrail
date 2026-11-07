import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { projectKeys } from './projects'

export type CloneDomains = 'none' | 'placeholder'

export interface ProjectCloneRequest {
  name: string
  environment?: string
  include: { env_vars: boolean; domains: CloneDomains; settings: boolean }
  dry_run: boolean
}

export interface ProjectClonePlan {
  dry_run: boolean
  applied: boolean
  source: { id?: string; name: string }
  project: { id?: string; name: string }
  environment?: string
  apps: { source: string; name: string; domains: string[]; env_vars: number }[]
  env_var_count: number
  secret_keys_needing_values: { scope: string; key: string }[]
  skipped: { scope: string; item: string; reason: string }[]
}

// POST /api/v1/projects/{id}/clone: one call for both the dry run and the apply.
export function useCloneProject(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation<ProjectClonePlan, ApiError, ProjectCloneRequest>({
    meta: { errorToast: false },
    mutationFn: async (body) => {
      const res = await fetch(
        `/api/v1/projects/${encodeURIComponent(projectId)}/clone`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `clone project failed: ${res.status}`),
        )
      }
      return (await res.json()) as ProjectClonePlan
    },
    onSuccess: (plan) => {
      if (!plan.applied) return
      void queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      void queryClient.invalidateQueries({ queryKey: appKeys.all })
    },
  })
}
