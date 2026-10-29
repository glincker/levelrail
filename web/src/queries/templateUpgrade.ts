// Template upgrade contract: GET and POST /apps/{name}/template-upgrade and
// GET /apps/{name}/template-upgrade/run. The plan resolves to null on any
// non-2xx because most apps are not template apps.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type UpgradeRisk = 'none' | 'low' | 'medium' | 'high'

export interface UpgradeBlocker {
  code: string
  service: string
  detail: string
}

export interface UpgradeServicePlan {
  name: string
  status: string
  image?: { from?: string; to?: string; kind: string; preserved?: boolean }
  env?: ReadonlyArray<{ name: string; action: string }>
  volumes?: ReadonlyArray<{ name: string; action: string }>
  ports?: ReadonlyArray<{ port: number; action: string }>
}

export interface TemplateUpgradePlan {
  app_id: string
  template_id: string
  risk: UpgradeRisk
  has_changes: boolean
  automatic: boolean
  blockers: ReadonlyArray<UpgradeBlocker>
  services: ReadonlyArray<UpgradeServicePlan>
  recheck_seconds: number
}

export type UpgradeRunState =
  'watching' | 'succeeded' | 'rolled_back' | 'rollback_failed' | 'refused'

export interface TemplateUpgradeRun {
  app_id: string
  state: UpgradeRunState
  message?: string
  from_revision: number
  to_revision: number
  rollback_revision?: number
}

const RUN_POLL_MS = 3_000
const DEFAULT_RECHECK_MS = 3_600_000

export const templateUpgradeKeys = {
  plan: (app: string) => ['apps', app, 'template-upgrade'] as const,
  run: (app: string) => ['apps', app, 'template-upgrade', 'run'] as const,
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/template-upgrade`
}

async function fetchPlan(app: string): Promise<TemplateUpgradePlan | null> {
  const res = await fetch(base(app))
  if (!res.ok) return null
  return (await res.json()) as TemplateUpgradePlan
}

async function fetchRun(app: string): Promise<TemplateUpgradeRun | null> {
  const res = await fetch(`${base(app)}/run`)
  if (!res.ok) return null
  return (await res.json()) as TemplateUpgradeRun
}

export function useTemplateUpgradePlan(app: string) {
  return useQuery({
    queryKey: templateUpgradeKeys.plan(app),
    queryFn: () => fetchPlan(app),
    retry: false,
    staleTime: 60_000,
    refetchInterval: (query) =>
      (query.state.data?.recheck_seconds ?? 0) * 1000 || DEFAULT_RECHECK_MS,
  })
}

export function useTemplateUpgradeRun(app: string, enabled: boolean) {
  return useQuery({
    queryKey: templateUpgradeKeys.run(app),
    queryFn: () => fetchRun(app),
    enabled,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.state === 'watching' ? RUN_POLL_MS : false,
  })
}

export function useApplyTemplateUpgrade(app: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (acceptRisk: boolean): Promise<TemplateUpgradeRun> => {
      const res = await fetch(base(app), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ accept_risk: acceptRisk }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `template upgrade failed: ${res.status}`),
        )
      }
      return (await res.json()) as TemplateUpgradeRun
    },
    onSuccess: (run) => {
      queryClient.setQueryData(templateUpgradeKeys.run(app), run)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: templateUpgradeKeys.plan(app),
      })
    },
  })
}
