// GET/POST /api/v1/settings/ingress/https (internal/api/ingress_https.go):
// the one-click "Enable HTTPS" flow for the dashboard's sslip.io hostname.
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { ingressSettingsKeys } from './domains'

export type HttpsState = 'off' | 'pending' | 'issued' | 'failed'
export type HttpsHint =
  'rate_limited' | 'unreachable' | 'dns' | 'caa' | 'unknown'

export interface HttpsStatus {
  state: HttpsState
  domain?: string
  public_host?: string
  public_host_source?: string
  suggested_domain?: string
  staging: boolean
  issuer?: string
  not_after?: string
  error?: string
  hint?: HttpsHint
  dashboard_url?: string
}

export interface EnableHttpsRequest {
  email: string
  staging: boolean
}

export const httpsStatusKey = ['ingress-settings', 'https'] as const

// Poll while a certificate is being requested; stop once it settles.
const HTTPS_PENDING_POLL_MS = 3_000

export async function fetchHttpsStatus(): Promise<HttpsStatus> {
  const res = await fetch('/api/v1/settings/ingress/https')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch https status failed: ${res.status}`),
    )
  }
  return (await res.json()) as HttpsStatus
}

export function httpsStatusQueryOptions() {
  return queryOptions({
    queryKey: httpsStatusKey,
    queryFn: fetchHttpsStatus,
    refetchInterval: (query) =>
      query.state.data?.state === 'pending' ? HTTPS_PENDING_POLL_MS : false,
  })
}

export async function enableHttps(
  req: EnableHttpsRequest,
): Promise<HttpsStatus> {
  const res = await fetch('/api/v1/settings/ingress/https', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `enable https failed: ${res.status}`),
    )
  }
  return (await res.json()) as HttpsStatus
}

export function useEnableHttps() {
  const queryClient = useQueryClient()
  return useMutation<HttpsStatus, ApiError, EnableHttpsRequest>({
    mutationFn: enableHttps,
    onSuccess: (status) => {
      queryClient.setQueryData(httpsStatusKey, status)
      void queryClient.invalidateQueries({ queryKey: ingressSettingsKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['certificates'] })
    },
  })
}
