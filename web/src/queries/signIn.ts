import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { RateLimitError, type LoginResult } from './auth'

// Sign in with a code, new-browser approval and trusted browsers
// (internal/api/login_code.go, login_approval.go, sign_in_requests.go).

export interface SignInCodeRequest {
  id: string
  requester_ip: string
  user_agent: string
  created_at: string
  expires_at: string
  revealable: boolean
}

export interface SignInApprovalRequest {
  id: string
  requester_ip: string
  user_agent: string
  created_at: string
  expires_at: string
}

export interface SignInRequests {
  codes: SignInCodeRequest[]
  approvals: SignInApprovalRequest[]
}

export interface TrustedDevice {
  id: string
  label: string
  ip: string
  created_at: string
  last_used_at: string
  expires_at: string
  current: boolean
}

export interface CodeLoginSettings {
  admins: boolean
  others: boolean
  saved: boolean
  new_device_approval: boolean
}

export type ApprovalPollStatus = 'pending' | 'approved' | 'denied' | 'expired'

export interface ApprovalPoll {
  status: ApprovalPollStatus
  expires_at?: string
  match_number?: number
  username?: string
  display_name?: string
  mfa_required?: boolean
  mfa_token?: string
}

export const signInKeys = {
  options: ['auth', 'login-options'] as const,
  requests: ['auth', 'sign-in-requests'] as const,
  devices: ['auth', 'trusted-devices'] as const,
  settings: ['settings', 'code-login'] as const,
}

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  // The public sign-in POSTs refuse anything but JSON, so a POST always sends it.
  const payload = body === undefined && method === 'POST' ? {} : body
  const res = await fetch(path, {
    method,
    headers:
      payload === undefined
        ? undefined
        : { 'Content-Type': 'application/json' },
    body: payload === undefined ? undefined : JSON.stringify(payload),
  })
  if (!res.ok) {
    const message = await readErrorMessage(res, `${path} failed: ${res.status}`)
    if (res.status === 429) {
      const parsed = Number.parseInt(res.headers.get('Retry-After') ?? '', 10)
      throw new RateLimitError(Number.isFinite(parsed) ? parsed : 0, message)
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export function loginOptionsQueryOptions() {
  return queryOptions({
    queryKey: signInKeys.options,
    queryFn: () =>
      send<{ code_login: boolean; trusted_device_days: number }>(
        'GET',
        '/api/v1/auth/login-options',
      ),
    staleTime: 60_000,
  })
}

export function requestLoginCode(username: string) {
  return send<{ message: string; expires_in: number }>(
    'POST',
    '/api/v1/auth/login-code/request',
    { username },
  )
}

export function redeemLoginCode(code: string) {
  return send<LoginResult>('POST', '/api/v1/auth/login-code/redeem', { code })
}

export function pollLoginApproval() {
  return send<ApprovalPoll>('POST', '/api/v1/auth/login-approval/poll')
}

export function signInRequestsQueryOptions() {
  return queryOptions({
    queryKey: signInKeys.requests,
    queryFn: () => send<SignInRequests>('GET', '/api/v1/auth/sign-in-requests'),
    refetchInterval: 10_000,
  })
}

export function revealLoginCode(id: string) {
  return send<{ code: string; expires_at: string }>(
    'POST',
    `/api/v1/auth/sign-in-requests/codes/${encodeURIComponent(id)}/reveal`,
  )
}

export function useDecideApproval() {
  const qc = useQueryClient()
  return useMutation<
    undefined,
    ApiError,
    { id: string; approve: boolean; match?: number }
  >({
    mutationFn: ({ id, approve, match }) =>
      send(
        'POST',
        `/api/v1/auth/login-approvals/${encodeURIComponent(id)}/${approve ? 'approve' : 'deny'}`,
        approve ? { match } : undefined,
      ),
    onSettled: () => qc.invalidateQueries({ queryKey: signInKeys.requests }),
  })
}

export function trustedDevicesQueryOptions() {
  return queryOptions({
    queryKey: signInKeys.devices,
    queryFn: async () =>
      (
        await send<{ devices: TrustedDevice[] }>(
          'GET',
          '/api/v1/auth/trusted-devices',
        )
      ).devices,
  })
}

export function useRevokeTrustedDevice() {
  const qc = useQueryClient()
  return useMutation<undefined, ApiError, string>({
    mutationFn: (id) =>
      send('DELETE', `/api/v1/auth/trusted-devices/${encodeURIComponent(id)}`),
    onSettled: () => qc.invalidateQueries({ queryKey: signInKeys.devices }),
  })
}

export function codeLoginSettingsQueryOptions() {
  return queryOptions({
    queryKey: signInKeys.settings,
    queryFn: () =>
      send<CodeLoginSettings>('GET', '/api/v1/settings/auth/code-login'),
  })
}

export function useUpdateCodeLoginSettings() {
  const qc = useQueryClient()
  return useMutation<
    CodeLoginSettings,
    ApiError,
    Partial<Pick<CodeLoginSettings, 'admins' | 'others'>>
  >({
    mutationFn: (body) =>
      send<CodeLoginSettings>('PUT', '/api/v1/settings/auth/code-login', body),
    onSuccess: (data) => {
      qc.setQueryData(signInKeys.settings, data)
    },
  })
}
