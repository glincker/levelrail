import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Recommended abuse shield for an app (internal/api/app_shield_recommended.go).

export interface ShieldRecommended {
  dry_run: boolean
  blocked?: string
  rate_per_minute: number
  burst: number
  max_concurrent: number
  block_bots: boolean
  applied: string[]
  skipped: { domain: string; reason: string }[]
}

export function useRecommendedShield(appName: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      dryRun: boolean
      blockBots: boolean
    }): Promise<ShieldRecommended> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/shield/recommended`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            dry_run: input.dryRun,
            block_bots: input.blockBots,
          }),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `recommended shield failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as ShieldRecommended
    },
    onSuccess: (data) => {
      if (!data.dry_run && data.applied.length > 0) {
        void qc.invalidateQueries({ queryKey: ['apps', appName] })
      }
    },
  })
}
