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
  accent?: string
  logo_url?: string
  about?: string
  website?: string
  noindex?: boolean
  powered_by?: boolean
  public_path?: string
}

export interface StatusGroup {
  id?: string
  name: string
  position: number
}

export type StatusComponentKind = 'app' | 'domain' | 'check'

export interface StatusComponent {
  id?: string
  kind: StatusComponentKind
  target: string
  display_name: string
  position: number
  group_id?: string
  description?: string
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
  groups: () => [...statusPageKeys.all, 'groups'] as const,
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

export function useStatusGroups() {
  return useQuery({
    queryKey: statusPageKeys.groups(),
    queryFn: () => request<StatusGroup[]>('/groups', 'list groups'),
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

export function useCreateStatusGroup() {
  return useStatusMutation((g: StatusGroup) =>
    request<StatusGroup>('/groups', 'add group', send('POST', g)),
  )
}

export function useUpdateStatusGroup() {
  return useStatusMutation((g: StatusGroup) =>
    request<StatusGroup>(
      `/groups/${encodeURIComponent(g.id ?? '')}`,
      'update group',
      send('PUT', g),
    ),
  )
}

export function useDeleteStatusGroup() {
  return useStatusMutation((id: string) =>
    request<void>(
      `/groups/${encodeURIComponent(id)}`,
      'delete group',
      send('DELETE'),
    ),
  )
}

export function useUpdateStatusComponent() {
  return useStatusMutation((c: StatusComponent) =>
    request<StatusComponent>(
      `/components/${encodeURIComponent(c.id ?? '')}`,
      'update component',
      send('PUT', c),
    ),
  )
}

export const WEBHOOK_EVENTS = [
  'incident_opened',
  'incident_updated',
  'incident_resolved',
  'component_changed',
  'maintenance_scheduled',
  'maintenance_updated',
  'maintenance_completed',
] as const

export interface StatusWebhook {
  id: string
  url: string
  label: string
  events: string[]
  paused: boolean
  paused_reason?: string
  consecutive_failures: number
  created_at: string
  // Present only on the create and rotate responses.
  signing_secret?: string
}

export interface StatusWebhookDelivery {
  id: string
  event_id: string
  event_type: string
  state: string
  attempts: number
  status_code: number
  last_error?: string
  created_at: string
}

export interface StatusWebhookTestResult {
  delivered: boolean
  status_code: number
  error?: string
}

export const webhookKeys = {
  list: () => [...statusPageKeys.all, 'webhooks'] as const,
  deliveries: (id: string) =>
    [...statusPageKeys.all, 'webhooks', id, 'deliveries'] as const,
}

export function useStatusWebhooks() {
  return useQuery({
    queryKey: webhookKeys.list(),
    queryFn: () => request<StatusWebhook[]>('/webhooks', 'list webhooks'),
  })
}

export function useStatusWebhookDeliveries(id: string, enabled: boolean) {
  return useQuery({
    queryKey: webhookKeys.deliveries(id),
    enabled,
    queryFn: () =>
      request<StatusWebhookDelivery[]>(
        `/webhooks/${encodeURIComponent(id)}/deliveries`,
        'load delivery log',
      ),
  })
}

export function useCreateStatusWebhook() {
  return useStatusMutation(
    (w: { url: string; label: string; events: string[] }) =>
      request<StatusWebhook>('/webhooks', 'add webhook', send('POST', w)),
  )
}

export function useStatusWebhookAction() {
  return useStatusMutation(
    ({ id, action }: { id: string; action: 'rotate' | 'pause' | 'resume' }) =>
      request<StatusWebhook>(
        `/webhooks/${encodeURIComponent(id)}/${action}`,
        `${action} webhook`,
        send('POST'),
      ),
  )
}

export function useTestStatusWebhook() {
  return useStatusMutation((id: string) =>
    request<StatusWebhookTestResult>(
      `/webhooks/${encodeURIComponent(id)}/test`,
      'send test ping',
      send('POST'),
    ),
  )
}

export function useDeleteStatusWebhook() {
  return useStatusMutation((id: string) =>
    request<void>(
      `/webhooks/${encodeURIComponent(id)}`,
      'delete webhook',
      send('DELETE'),
    ),
  )
}
