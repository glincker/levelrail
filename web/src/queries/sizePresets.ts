// GET /api/v1/size-presets: the named CPU and memory pairs an operator
// configures in settings. A plain query so a card renders without them.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface SizePreset {
  name: string
  cpu: number
  memory_bytes: number
}

export async function fetchSizePresets(): Promise<SizePreset[]> {
  const res = await fetch('/api/v1/size-presets')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch size presets failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { presets: SizePreset[] }
  return body.presets
}

export function sizePresetsQueryOptions() {
  return queryOptions({
    queryKey: ['size-presets'],
    queryFn: fetchSizePresets,
    staleTime: 60_000,
  })
}

export function useSizePresets() {
  return useQuery(sizePresetsQueryOptions())
}
