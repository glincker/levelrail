// GET /api/v1/apps/{name}/edge and POST /api/v1/apps/{name}/cdn/setup:
// the one-line git to edge pipeline and the CDN setup report.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

const EDGE_POLL_MS = Number(import.meta.env.VITE_EDGE_POLL_MS) || 30_000

export type EdgeHealth = 'direct' | 'ok' | 'attention'

export interface EdgePurge {
  at: string
  trigger: string
  scope: string
  provider: string
  zones?: string[]
  hosts?: string[] | number
  ok: boolean
  error?: string
}

export interface EdgeDeploy {
  id: string
  status: string
  source?: string
  commit_sha?: string
  started_at?: string
  finished_at?: string
}

export interface EdgeStatus {
  app: string
  health: EdgeHealth
  proxied: boolean
  override: boolean | null
  provider?: string
  zone?: string
  ssl_mode?: string
  ssl_warning?: string
  domains?: string[]
  deploy?: EdgeDeploy | null
  dns?: { managed_records: number }
  cache?: {
    hits: number
    misses: number
    hit_rate: number
    entries: number
  }
  last_purge?: EdgePurge | null
  purges?: EdgePurge[]
  issues?: string[]
}

export type CdnStepStatus = 'ok' | 'warn' | 'fail' | 'skipped'

export interface CdnStep {
  id: string
  status: CdnStepStatus
  message: string
  fix?: string
}

export interface CdnSetupResult {
  app: string
  provider: string
  dry_run: boolean
  applied: boolean
  status: 'ok' | 'warn' | 'fail'
  steps: CdnStep[]
}

export interface CdnSetupRequest {
  provider?: string
  dry_run?: boolean
}

export const appEdgeKeys = {
  detail: (name: string) => ['apps', name, 'edge'] as const,
}

export function appEdgeQueryOptions(name: string) {
  return queryOptions({
    queryKey: appEdgeKeys.detail(name),
    queryFn: async (): Promise<EdgeStatus> => {
      const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/edge`)
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch edge status failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as EdgeStatus
    },
    refetchInterval: EDGE_POLL_MS,
    retry: false,
  })
}

/** Never throws into the overview: the card hides itself on error. */
export function useAppEdge(name: string) {
  return useQuery(appEdgeQueryOptions(name))
}

export function useCdnSetup(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: async (req: CdnSetupRequest): Promise<CdnSetupResult> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(name)}/cdn/setup`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `cdn setup failed: ${res.status}`),
        )
      }
      return (await res.json()) as CdnSetupResult
    },
    onSuccess: (result) => {
      if (result.applied) {
        void queryClient.invalidateQueries({
          queryKey: appEdgeKeys.detail(name),
        })
      }
    },
  })
}
