// GET /apps/{name}/slo-preview (internal/api/slo.go): error budget left and
// the burn rate per alert tier for a candidate SLO.

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { SloConfig, SloObjective } from '../types/alerts'

export interface SloWindowStat {
  window_seconds: number
  requests: number
  bad: number
  burn_rate: number
}

export interface SloTierStatus {
  name: string
  factor: number
  long_seconds: number
  short_seconds: number
  page: boolean
  long_burn: number
  short_burn: number
  effective_burn: number
  firing: boolean
}

export interface SloPreview {
  config: SloConfig
  has_traffic: boolean
  budget_remaining: number
  budget_window_requests: number
  windows: SloWindowStat[]
  tiers: SloTierStatus[]
  firing: boolean
  page: boolean
  max_burn: number
}

export const DEFAULT_SLO_TARGET = 99.9

export const sloPreviewKeys = {
  one: (app: string, cfg: SloConfig) =>
    [
      'slo-preview',
      app,
      cfg.objective,
      cfg.target,
      cfg.latency_ms ?? 0,
    ] as const,
}

export async function fetchSloPreview(
  app: string,
  cfg: SloConfig,
): Promise<SloPreview | null> {
  const qs = new URLSearchParams({
    objective: cfg.objective,
    target: String(cfg.target),
  })
  if (cfg.objective === 'latency' && cfg.latency_ms) {
    qs.set('latency_ms', String(cfg.latency_ms))
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(app)}/slo-preview?${qs.toString()}`,
  )
  if (res.status === 501) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `slo preview failed: ${res.status}`),
    )
  }
  return (await res.json()) as SloPreview
}

export function isValidSlo(cfg: SloConfig): boolean {
  if (!(cfg.target >= 50 && cfg.target < 100)) return false
  if (cfg.objective === 'latency') return (cfg.latency_ms ?? 0) >= 5
  return true
}

export function useSloPreview(app: string, cfg: SloConfig, enabled = true) {
  return useQuery({
    queryKey: sloPreviewKeys.one(app, cfg),
    queryFn: () => fetchSloPreview(app, cfg),
    enabled: enabled && isValidSlo(cfg),
    retry: false,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })
}

export function sloConfigFromForm(
  objective: SloObjective,
  target: string | number,
  latencyMs: string | number,
): SloConfig {
  const cfg: SloConfig = { objective, target: Number(target) }
  if (objective === 'latency') cfg.latency_ms = Number(latencyMs)
  return cfg
}
