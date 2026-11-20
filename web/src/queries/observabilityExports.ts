// Presets catalog and export CRUD for /api/v1/observability
// (internal/api/observability.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '@/lib/pollTier'

export type ExportSignalName = 'logs' | 'metrics'
export type ExportFix =
  'bad_token' | 'tls_error' | 'unreachable' | 'rejected' | 'private_blocked'

export interface PresetChoice {
  value: string
  label: string
}

export interface PresetField {
  key: string
  label: string
  type?: string
  secret?: boolean
  required?: boolean
  default?: string
  placeholder?: string
  help?: string
  choices?: PresetChoice[]
}

export interface PresetSignal {
  signal: ExportSignalName
  default: boolean
}

export interface ExportPreset {
  id: string
  name: string
  description: string
  docs_anchor: string
  fields: PresetField[]
  signals: PresetSignal[]
}

export interface ExportSignalStatus {
  signal: ExportSignalName
  destination_id: string
  kind: string
  endpoint: string
  enabled: boolean
  header_names: string[]
  has_credential: boolean
  last_success_at?: string
  last_error?: string
  sent_total: number
  circuit_open: boolean
}

export interface ObservabilityExport {
  id: string
  preset: string
  preset_name: string
  signals: ExportSignalStatus[]
}

export interface CreateExportRequest {
  preset: string
  name: string
  fields: Record<string, string>
  signals?: ExportSignalName[]
}

export interface ExportTestSignalResult {
  signal: ExportSignalName
  ok: boolean
  status?: number
  error?: string
  hint?: string
  fix?: ExportFix
}

export interface ExportTestResult {
  id: string
  ok: boolean
  results: ExportTestSignalResult[]
}

export const observabilityExportKeys = {
  all: ['observability-exports'] as const,
  presets: () => [...observabilityExportKeys.all, 'presets'] as const,
  list: () => [...observabilityExportKeys.all, 'list'] as const,
}

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

export function useExportPresets() {
  return useQuery(
    queryOptions({
      queryKey: observabilityExportKeys.presets(),
      queryFn: async () =>
        (
          await request<{ presets: ExportPreset[] }>(
            '/api/v1/observability/presets',
            undefined,
            'fetch export presets',
          )
        ).presets,
      staleTime: 300_000,
      retry: false,
    }),
  )
}

export function useObservabilityExports() {
  return useQuery(
    queryOptions({
      queryKey: observabilityExportKeys.list(),
      queryFn: () =>
        request<ObservabilityExport[]>(
          '/api/v1/observability/exports',
          undefined,
          'fetch exports',
        ),
      refetchInterval: pollMs('active'),
      retry: false,
    }),
  )
}

export function useCreateObservabilityExport() {
  const qc = useQueryClient()
  return useMutation<ObservabilityExport, ApiError, CreateExportRequest>({
    mutationFn: (body) =>
      request<ObservabilityExport>(
        '/api/v1/observability/exports',
        jsonInit('POST', body),
        'create export',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: observabilityExportKeys.list() }),
  })
}

export function useTestObservabilityExport() {
  const qc = useQueryClient()
  return useMutation<ExportTestResult, ApiError, string>({
    mutationFn: (id) =>
      request<ExportTestResult>(
        `/api/v1/observability/exports/${encodeURIComponent(id)}/test`,
        jsonInit('POST'),
        'test export',
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: observabilityExportKeys.list() }),
  })
}

export function useDeleteObservabilityExport() {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) =>
      request<void>(
        `/api/v1/observability/exports/${encodeURIComponent(id)}`,
        jsonInit('DELETE'),
        'delete export',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: observabilityExportKeys.list() }),
  })
}
