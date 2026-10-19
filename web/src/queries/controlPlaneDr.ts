import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ControlPlaneDrWarning {
  code: string
  message: string
}

export interface ControlPlaneDrDrill {
  at?: string
  ok: boolean
  partial: boolean
  detail: string
  duration_ms: number
}

export interface ControlPlaneDrChecklist {
  destination_chosen: boolean
  recipient_set: boolean
  escrow_acknowledged: boolean
  drill_passed: boolean
}

export interface ControlPlaneDr {
  enabled: boolean
  configured: boolean
  target_id: string
  target_name?: string
  install_id: string
  recipients: string[]
  schedule: string
  drill_schedule: string
  retain_daily: number
  retain_weekly: number
  retain_monthly: number
  escrow_target_id: string
  escrow_generated_at?: string
  escrow_acked_at?: string
  next_backup_at?: string
  next_drill_at?: string
  last_backup_at?: string
  last_backup_key?: string
  last_backup_error?: string
  last_attempt_at?: string
  last_drill: ControlPlaneDrDrill
  drill_identity_configured: boolean
  backup_running: boolean
  drill_running: boolean
  checklist: ControlPlaneDrChecklist
  warnings: ControlPlaneDrWarning[]
  replica?: ControlPlaneDrReplica
}

export interface ControlPlaneDrReplica {
  enabled: boolean
  interval_seconds: number
  last_ship_at?: string
  last_ship_ms?: number
  last_size_bytes?: number
  last_error?: string
  pending_since?: string
}

export interface ControlPlaneDrSettings {
  enabled: boolean
  target_id: string
  recipients: string[]
  schedule: string
  drill_schedule: string
  retain_daily: number
  retain_weekly: number
  retain_monthly: number
  escrow_target_id: string
}

export interface ControlPlaneEscrowBundle {
  armored: string
  instructions: string
  fingerprint: string
  recipient_count: number
  created_at: string
  uploaded_key?: string
}

export interface ControlPlaneRestoreStep {
  id: string
  state: 'done' | 'todo' | 'blocked'
}

export interface ControlPlaneRestorePlan {
  ready: boolean
  backup?: { key: string; created_at: string; size_bytes: number }
  install_id?: string
  source?: string
  endpoint?: string
  command?: string
  steps: ControlPlaneRestoreStep[]
  warnings: string[]
  binary_name: string
}

export interface ControlPlaneDrKeypair {
  recipient: string
  identity: string
}

export const controlPlaneDrKeys = {
  all: ['control-plane-dr'] as const,
  status: () => [...controlPlaneDrKeys.all, 'status'] as const,
  restorePlan: () => [...controlPlaneDrKeys.all, 'restore-plan'] as const,
}

const BASE = '/api/v1/system/control-plane-dr'

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(BASE + path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

export function useControlPlaneDr() {
  return useQuery({
    queryKey: controlPlaneDrKeys.status(),
    queryFn: () => request<ControlPlaneDr>('', undefined, 'load status'),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.backup_running || query.state.data?.drill_running
        ? 2000
        : false,
  })
}

function useInvalidating<TVars, TData>(fn: (vars: TVars) => Promise<TData>) {
  const qc = useQueryClient()
  return useMutation<TData, Error, TVars>({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: controlPlaneDrKeys.all }),
  })
}

export function useUpdateControlPlaneDr() {
  return useInvalidating((s: ControlPlaneDrSettings) =>
    request<ControlPlaneDr>('/settings', jsonInit('PUT', s), 'save settings'),
  )
}

export function useRunControlPlaneDrBackup() {
  return useInvalidating(() =>
    request<{ started: boolean }>('/run', jsonInit('POST'), 'start backup'),
  )
}

export function useRunControlPlaneDrDrill() {
  return useInvalidating(() =>
    request<{ started: boolean }>('/drill', jsonInit('POST'), 'start drill'),
  )
}

export function useBuildControlPlaneEscrow() {
  return useInvalidating((req: { recipients: string[]; upload: boolean }) =>
    request<ControlPlaneEscrowBundle>(
      '/escrow',
      jsonInit('POST', req),
      'build escrow bundle',
    ),
  )
}

export function useAckControlPlaneEscrow() {
  return useInvalidating(() =>
    request<ControlPlaneDr>('/escrow/ack', jsonInit('POST'), 'acknowledge'),
  )
}

export function useControlPlaneRestorePlan(enabled: boolean) {
  return useQuery({
    queryKey: controlPlaneDrKeys.restorePlan(),
    queryFn: () =>
      request<ControlPlaneRestorePlan>(
        '/restore-plan',
        undefined,
        'load restore plan',
      ),
    enabled,
    retry: false,
  })
}

export function useGenerateControlPlaneDrKeypair() {
  return useMutation<ControlPlaneDrKeypair, Error, void>({
    mutationFn: () =>
      request<ControlPlaneDrKeypair>(
        '/keys',
        jsonInit('POST'),
        'generate key pair',
      ),
  })
}
