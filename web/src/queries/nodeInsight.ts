import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '../lib/pollTier'

export type TimelineWindow = '24h' | '168h' | '720h'

export interface NodeTimelineSegment {
  state: string
  from: string
  to: string
  seconds: number
}

export interface NodeTimeline {
  from: string
  to: string
  segments: NodeTimelineSegment[]
  transitions: number
  uptime_percent?: number
}

export interface NodeDoctorCheck {
  id: string
  status: 'pass' | 'warn' | 'fail' | 'unknown'
  detail: string
  fix?: string
  fix_command?: string
}

export interface NodeDoctor {
  node_id: string
  reported: boolean
  checks: NodeDoctorCheck[]
  summary: { pass: number; warn: number; fail: number; unknown: number }
}

export interface NodeSkewRow {
  id: string
  name: string
  version?: string
  os?: string
  arch?: string
  online: boolean
  class: 'current' | 'behind' | 'ahead' | 'unknown'
  needs_update: boolean
}

export interface NodeUpgradeStep {
  order: number
  node_id: string
  name: string
  from?: string
  to: string
  note?: string
}

export interface NodeVersionSkew {
  control_plane_version: string
  nodes: NodeSkewRow[]
  current: number
  behind: number
  ahead: number
  unknown: number
  plan: NodeUpgradeStep[]
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export const nodeInsightKeys = {
  timeline: (id: string, window: TimelineWindow) =>
    ['nodes', 'detail', id, 'timeline', window] as const,
  doctor: (id: string) => ['nodes', 'detail', id, 'doctor'] as const,
  skew: () => ['nodes', 'version-skew'] as const,
}

export function useNodeTimeline(id: string, window: TimelineWindow) {
  return useQuery({
    queryKey: nodeInsightKeys.timeline(id, window),
    queryFn: () =>
      getJson<NodeTimeline>(
        `/api/v1/nodes/${encodeURIComponent(id)}/timeline?window=${window}`,
        'fetch node timeline',
      ),
    retry: false,
    refetchInterval: pollMs('steady'),
  })
}

export function useNodeDoctor(id: string, enabled: boolean) {
  return useQuery({
    queryKey: nodeInsightKeys.doctor(id),
    queryFn: () =>
      getJson<NodeDoctor>(
        `/api/v1/nodes/${encodeURIComponent(id)}/doctor`,
        'fetch node doctor',
      ),
    enabled,
    retry: false,
  })
}

export function useNodeVersionSkew() {
  return useQuery({
    queryKey: nodeInsightKeys.skew(),
    queryFn: () =>
      getJson<NodeVersionSkew>(
        '/api/v1/nodes/version-skew',
        'fetch node version skew',
      ),
    retry: false,
    refetchInterval: pollMs('steady'),
  })
}
