import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AlertHistoryEntry } from '../types/alertNoise'

// Matches internal/api's inboxItemResource and inboxPrefsResource.
export interface InboxItem extends AlertHistoryEntry {
  read: boolean
}

export interface InboxPrefs {
  min_severity: 'info' | 'warning' | 'critical'
  muted_apps: string[]
  digest_enabled: boolean
  digest_hour: number
  digest_channel_id: string
  last_digest_at?: string
}

export interface InboxState {
  read_ids: string[]
  archived_ids: string[]
}

export const inboxKeys = {
  all: ['inbox'] as const,
  list: (unreadOnly: boolean, archived = false) =>
    [...inboxKeys.all, 'list', unreadOnly, archived] as const,
  state: () => [...inboxKeys.all, 'state'] as const,
  count: () => [...inboxKeys.all, 'count'] as const,
  prefs: () => [...inboxKeys.all, 'prefs'] as const,
}

async function request<T>(
  url: string,
  what: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const JSON_HEADERS = { 'Content-Type': 'application/json' }

export function useInbox(unreadOnly: boolean, archived = false) {
  return useQuery({
    queryKey: inboxKeys.list(unreadOnly, archived),
    queryFn: ({ signal }) =>
      request<InboxItem[]>(
        `/api/v1/inbox?limit=50${unreadOnly ? '&unread=true' : ''}${archived ? '&archived=true' : ''}`,
        'load inbox',
        { signal },
      ),
    retry: false,
    refetchInterval: 30_000,
  })
}

export function useInboxUnreadCount() {
  return useQuery({
    queryKey: inboxKeys.count(),
    queryFn: ({ signal }) =>
      request<{ unread: number }>(
        '/api/v1/inbox/unread-count',
        'load unread count',
        { signal },
      ),
    retry: false,
    refetchInterval: 30_000,
    select: (d) => d.unread,
  })
}

function invalidateInbox(qc: QueryClient) {
  return qc.invalidateQueries({ queryKey: inboxKeys.all })
}

export function useMarkInboxRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { ids?: string[]; all?: boolean }) =>
      request<void>('/api/v1/inbox/read', 'mark read', {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(body),
      }),
    onSuccess: () => invalidateInbox(qc),
  })
}

export function useInboxState() {
  return useQuery({
    queryKey: inboxKeys.state(),
    queryFn: ({ signal }) =>
      request<InboxState>('/api/v1/inbox/state', 'load inbox state', {
        signal,
      }),
    retry: false,
    refetchInterval: 30_000,
  })
}

export function useArchiveInbox() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { ids: string[]; restore?: boolean }) =>
      request<void>('/api/v1/inbox/archive', 'archive', {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(body),
      }),
    onSuccess: () => invalidateInbox(qc),
  })
}

export function useInboxPrefs() {
  return useQuery({
    queryKey: inboxKeys.prefs(),
    queryFn: ({ signal }) =>
      request<InboxPrefs>('/api/v1/inbox/preferences', 'load preferences', {
        signal,
      }),
    retry: false,
  })
}

export function useSaveInboxPrefs() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (prefs: InboxPrefs) =>
      request<InboxPrefs>('/api/v1/inbox/preferences', 'save preferences', {
        method: 'PUT',
        headers: JSON_HEADERS,
        body: JSON.stringify(prefs),
      }),
    onSuccess: () => invalidateInbox(qc),
  })
}
