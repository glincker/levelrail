// GET /api/v1/health-presets (handleGetHealthPresets): the one preset list the CLI shares.
import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface HealthPreset {
  id: string
  path: string
  hint: string
}

export interface HealthPresetTiming {
  interval_seconds: number
  timeout_seconds: number
  failures: number
}

export interface HealthPresetCatalog {
  presets: HealthPreset[]
  readiness: HealthPresetTiming
  liveness: HealthPresetTiming
}

export function healthPresetsQueryOptions() {
  return queryOptions({
    queryKey: ['health-presets'] as const,
    queryFn: async (): Promise<HealthPresetCatalog> => {
      const res = await fetch('/api/v1/health-presets')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `load health presets failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as HealthPresetCatalog
    },
    staleTime: Infinity,
  })
}

/** Empty until the catalog loads; the custom path field always works without it. */
export function useHealthPresets(): HealthPresetCatalog | undefined {
  return useQuery({ ...healthPresetsQueryOptions(), retry: false }).data
}
