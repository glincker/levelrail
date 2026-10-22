// GET/PUT/DELETE and POST .../test on /api/v1/settings/google-cloud-dns
// (internal/api/google_cloud_dns.go). The service account JSON is write-only.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

const PATH = '/api/v1/settings/google-cloud-dns'

export const googleCloudDnsKeys = {
  all: ['google-cloud-dns'] as const,
}

export interface GoogleCloudDnsSettings {
  enabled: boolean
  project_id?: string
  client_email?: string
  has_key: boolean
}

export interface UpdateGoogleCloudDnsRequest {
  enabled: boolean
  // Omitted keeps the stored key.
  service_account_json?: string
}

export interface GoogleCloudDnsTestResult {
  ok: boolean
  zones?: string[]
  error?: string
}

async function send<T>(
  path: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function googleCloudDnsQueryOptions() {
  return queryOptions({
    queryKey: googleCloudDnsKeys.all,
    queryFn: () => send<GoogleCloudDnsSettings>(PATH, 'GET'),
    staleTime: 60_000,
  })
}

export function useGoogleCloudDnsSettings() {
  return useQuery(googleCloudDnsQueryOptions())
}

export function useUpdateGoogleCloudDns() {
  const queryClient = useQueryClient()
  return useMutation<
    GoogleCloudDnsSettings,
    ApiError,
    UpdateGoogleCloudDnsRequest
  >({
    mutationFn: (req) => send<GoogleCloudDnsSettings>(PATH, 'PUT', req),
    onSuccess: (updated) => {
      queryClient.setQueryData(googleCloudDnsKeys.all, updated)
    },
  })
}

export function useDisconnectGoogleCloudDns() {
  const queryClient = useQueryClient()
  return useMutation<GoogleCloudDnsSettings, ApiError, void>({
    mutationFn: () => send<GoogleCloudDnsSettings>(PATH, 'DELETE'),
    onSuccess: (updated) => {
      queryClient.setQueryData(googleCloudDnsKeys.all, updated)
    },
  })
}

export function useTestGoogleCloudDns() {
  return useMutation<GoogleCloudDnsTestResult, ApiError, void>({
    mutationFn: () => send<GoogleCloudDnsTestResult>(`${PATH}/test`, 'POST'),
  })
}
