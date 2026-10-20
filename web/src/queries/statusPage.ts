// Types, query keys and hooks for the status page management routes
// (internal/api/status_page.go). The public page itself is server
// rendered and never goes through this module.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface StatusPageSettings {
  enabled: boolean
  title: string
  description: string
  custom_domain: string
  public_path?: string
}

export type StatusComponentKind = 'app' | 'domain' | 'check'

export interface StatusComponent {
  id?: string
  kind: StatusComponentKind
  target: string
  display_name: string
  position: number
}

export type IncidentKind = 'incident' | 'maintenance'
export type IncidentImpact = 'none' | 'minor' | 'major' | 'critical'

export interface StatusUpdate {
  status: string
  body: string
  created_at?: string
}

export interface StatusIncident {
  id?: string
  kind: IncidentKind
  title: string
  status: string
  impact: IncidentImpact
  component_ids: string[]
  starts_at?: string
  ends_at?: string
  resolved_at?: string
  body?: string
  updates?: StatusUpdate[]
}

export interface StatusPreview {
  title: string
  status: string
  status_text: string
  components: { name: string; status: string; uptime_90d: number | null }[]
}

export const INCIDENT_STATUSES: Record<IncidentKind, string[]> = {
  incident: ['investigating', 'identified', 'monitoring', 'resolved'],
  maintenance: ['scheduled', 'in_progress', 'completed'],
}

const base = '/api/v1/status-page'

export const statusPageKeys = {
  all: ['status-page'] as const,
  settings: () => [...statusPageKeys.all, 'settings'] as const,
  components: () => [...statusPageKeys.all, 'components'] as const,
  incidents: () => [...statusPageKeys.all, 'incidents'] as const,
  preview: () => [...statusPageKeys.all, 'preview'] as const,
}

async function request<T>(
  path: string,
  what: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(`${base}${path}`, init)
  if (res.status === 501) {
    throw new ApiError(
      501,
      'the status page is not configured on this control plane',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function send(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function useStatusPageSettings() {
  return useQuery({
    queryKey: statusPageKeys.settings(),
    queryFn: () => request<StatusPageSettings>('', 'load status page'),
  })
}

export function useStatusComponents() {
  return useQuery({
    queryKey: statusPageKeys.components(),
    queryFn: () => request<StatusComponent[]>('/components', 'list components'),
  })
}

export function useStatusIncidents() {
  return useQuery({
    queryKey: statusPageKeys.incidents(),
    queryFn: () => request<StatusIncident[]>('/incidents', 'list incidents'),
  })
}

export function useStatusPreview() {
  return useQuery({
    queryKey: statusPageKeys.preview(),
    queryFn: () => request<StatusPreview>('/preview', 'preview status page'),
  })
}

function useStatusMutation<V, R>(fn: (v: V) => Promise<R>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: statusPageKeys.all })
    },
  })
}

export function useSaveStatusPageSettings() {
  return useStatusMutation((s: StatusPageSettings) =>
    request<StatusPageSettings>('', 'save status page', send('PUT', s)),
  )
}

export function useCreateStatusComponent() {
  return useStatusMutation((c: StatusComponent) =>
    request<StatusComponent>('/components', 'add component', send('POST', c)),
  )
}

export function useDeleteStatusComponent() {
  return useStatusMutation((id: string) =>
    request<void>(
      `/components/${encodeURIComponent(id)}`,
      'delete component',
      send('DELETE'),
    ),
  )
}

export function useCreateStatusIncident() {
  return useStatusMutation((i: StatusIncident) =>
    request<StatusIncident>('/incidents', 'create incident', send('POST', i)),
  )
}

export function usePostStatusUpdate() {
  return useStatusMutation(
    ({ id, update }: { id: string; update: StatusUpdate }) =>
      request<StatusIncident>(
        `/incidents/${encodeURIComponent(id)}/updates`,
        'post update',
        send('POST', update),
      ),
  )
}

export function useDeleteStatusIncident() {
  return useStatusMutation((id: string) =>
    request<void>(
      `/incidents/${encodeURIComponent(id)}`,
      'delete incident',
      send('DELETE'),
    ),
  )
}
