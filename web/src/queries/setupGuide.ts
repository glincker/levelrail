// Get started hub: GET/PUT/POST /api/v1/setup/guide (internal/api/setup_guide.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  SetupGuide,
  SetupGuideOverride,
  SetupGuideStepId,
} from '../lib/setupGuide'

export const setupGuideKey = ['setup-guide'] as const

const FOCUS_REFETCH_MS = 5_000

async function readGuide(res: Response, what: string): Promise<SetupGuide> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as SetupGuide
}

export async function fetchSetupGuide(): Promise<SetupGuide> {
  return readGuide(await fetch('/api/v1/setup/guide'), 'fetch setup guide')
}

export function setupGuideQueryOptions(staleTime = 30_000) {
  return queryOptions({
    queryKey: setupGuideKey,
    queryFn: fetchSetupGuide,
    staleTime,
  })
}

/** useSetupGuide never throws: entry points must not break the page they sit on. */
export function useSetupGuide(options: { live?: boolean } = {}) {
  return useQuery({
    ...setupGuideQueryOptions(options.live ? 0 : 30_000),
    retry: false,
    refetchInterval: options.live ? FOCUS_REFETCH_MS : false,
    refetchOnWindowFocus: true,
  })
}

function useGuideMutation<V>(run: (v: V) => Promise<Response>, what: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (v: V) => readGuide(await run(v), what),
    onSuccess: (guide) => {
      queryClient.setQueryData(setupGuideKey, guide)
    },
  })
}

export function useSetGuideStep() {
  return useGuideMutation(
    (v: { id: SetupGuideStepId; state: SetupGuideOverride }) =>
      fetch(`/api/v1/setup/guide/steps/${encodeURIComponent(v.id)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ state: v.state }),
      }),
    'update setup step',
  )
}

export function useSetGuideHidden() {
  return useGuideMutation(
    (hidden: boolean) =>
      fetch('/api/v1/setup/guide/visibility', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ hidden }),
      }),
    'update setup guide visibility',
  )
}

export function useResetGuide() {
  return useGuideMutation(
    () => fetch('/api/v1/setup/guide/reset', { method: 'POST' }),
    'reset setup guide',
  )
}
