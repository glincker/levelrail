import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import { accountKeys } from './myAccess'
import { sessionKeys } from './security'

// Self-service account endpoints: internal/api/account_*.go.

export class AccountApiError extends ApiError {
  readonly code: string
  readonly body: unknown

  constructor(status: number, message: string, code: string, body: unknown) {
    super(status, message)
    this.name = 'AccountApiError'
    this.code = code
    this.body = body
  }
}

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    cache: 'no-store',
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const parsed = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new AccountApiError(
      res.status,
      parsed?.error ?? `${path} failed: ${String(res.status)}`,
      parsed?.code ?? '',
      parsed,
    )
  }
  if (res.status === 204 || res.status === 202) {
    const text = await res.text()
    return (text === '' ? undefined : JSON.parse(text)) as T
  }
  return (await res.json()) as T
}

export const accountSelfKeys = {
  activity: ['account', 'activity'] as const,
  preferences: ['account', 'preferences'] as const,
  deleteCheck: ['account', 'delete-check'] as const,
  emailChange: ['account', 'email-change'] as const,
}

export type ActivityKind =
  | 'sign_in'
  | 'security'
  | 'sessions'
  | 'tokens'
  | 'token_use'
  | 'account'
  | 'failed'
  | 'other'

export interface ActivityEntry {
  id: string
  at: string
  kind: ActivityKind
  event: string
  actor_type: string
  token_name?: string
  method: string
  path: string
  status_code: number
  remote_addr: string
  client_kind: string
}

interface ActivityPage {
  entries: ActivityEntry[]
  next_cursor?: string
}

export function activityQueryOptions(kind: ActivityKind | 'important' | 'all') {
  return infiniteQueryOptions({
    queryKey: [...accountSelfKeys.activity, kind],
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const q = new URLSearchParams({ limit: '25' })
      if (kind !== 'important') q.set('kind', kind)
      if (pageParam !== '') q.set('cursor', pageParam)
      return send<ActivityPage>('GET', `/api/v1/auth/my-activity?${q}`)
    },
    getNextPageParam: (last) => last.next_cursor,
  })
}

export interface Preferences {
  timezone: string
  language: string
  theme: '' | 'light' | 'dark' | 'system'
  landing_page: string
  gravatar: boolean
}

export function preferencesQueryOptions() {
  return queryOptions({
    queryKey: accountSelfKeys.preferences,
    queryFn: () => send<Preferences>('GET', '/api/v1/auth/preferences'),
    staleTime: 5 * 60_000,
  })
}

export function useSavePreferences() {
  const qc = useQueryClient()
  return useMutation<Preferences, AccountApiError, Preferences>({
    mutationFn: (next) => send('PUT', '/api/v1/auth/preferences', next),
    onSuccess: (saved) => {
      qc.setQueryData(accountSelfKeys.preferences, saved)
      void qc.invalidateQueries({ queryKey: accountSelfKeys.activity })
    },
  })
}

export interface AccountExportJob {
  id: string
  status: 'pending' | 'ready' | 'failed'
  size_bytes?: number
  expires_at: string
}

export function useStartAccountExport() {
  return useMutation<AccountExportJob, AccountApiError>({
    mutationFn: () => send('POST', '/api/v1/auth/export'),
  })
}

export function accountExportQueryOptions(id: string | null) {
  return queryOptions({
    queryKey: ['account', 'export', id] as const,
    enabled: id !== null,
    queryFn: () =>
      send<AccountExportJob>(
        'GET',
        `/api/v1/auth/export/${encodeURIComponent(id ?? '')}`,
      ),
    refetchInterval: (query) =>
      query.state.data?.status === 'pending' ? 1000 : false,
  })
}

export function accountExportDownloadUrl(id: string): string {
  return `/api/v1/auth/export/${encodeURIComponent(id)}/download`
}

export type DeleteBlockerCode = 'last_user' | 'last_root' | 'team_owner'

export interface DeleteBlocker {
  code: DeleteBlockerCode
  resource_id?: string
  resource_name?: string
  fix_path: string
}

export interface DeleteCheck {
  can_delete: boolean
  email: string
  blockers: DeleteBlocker[]
}

export function deleteCheckQueryOptions() {
  return queryOptions({
    queryKey: accountSelfKeys.deleteCheck,
    queryFn: () =>
      send<DeleteCheck>('GET', '/api/v1/auth/delete-account/check'),
  })
}

export function useDeleteAccount() {
  return useMutation<void, AccountApiError, { confirm: string }>({
    mutationFn: ({ confirm }) =>
      send('DELETE', '/api/v1/auth/account', { confirm }),
  })
}

export function useUnlinkProvider() {
  const qc = useQueryClient()
  return useMutation<void, AccountApiError, string>({
    mutationFn: (provider) =>
      send('DELETE', `/api/v1/auth/link/${encodeURIComponent(provider)}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: accountKeys.access })
      void qc.invalidateQueries({ queryKey: accountSelfKeys.activity })
    },
  })
}

export function emailChangeStatusQueryOptions() {
  return queryOptions({
    queryKey: accountSelfKeys.emailChange,
    queryFn: () =>
      send<{ available: boolean }>('GET', '/api/v1/auth/email-change'),
  })
}

export interface EmailChangeRequest {
  new_email: string
  password: string
  code?: string
}

export function useRequestEmailChange() {
  return useMutation<void, AccountApiError, EmailChangeRequest>({
    mutationFn: (req) => send('POST', '/api/v1/auth/email-change', req),
  })
}

export function useConfirmEmailChange() {
  const qc = useQueryClient()
  return useMutation<{ email: string }, AccountApiError, string>({
    mutationFn: (token) =>
      send('POST', '/api/v1/auth/email-change/confirm', { token }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: sessionKeys.all })
      void qc.invalidateQueries({ queryKey: accountSelfKeys.activity })
    },
  })
}
