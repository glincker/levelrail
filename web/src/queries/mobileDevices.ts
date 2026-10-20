import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'
import { tokenKeys } from './tokens'

export const MOBILE_PAIRING_STEP_UP_APP = 'mobile-pairing'

// Matches internal/api's mobileDeviceResource.
export interface MobileDevice {
  id: string
  name: string
  platform: string
  token_id: string
  created_at: string
  last_seen_at?: string
  expires_at?: string
  expired: boolean
}

// Matches internal/api's pairingCodeResponse.
export interface MobilePairingCode {
  code: string
  expires_at: string
  server_url: string
  pair_url: string
  qr_payload: string
}

export const mobileDeviceKeys = {
  all: ['mobile-devices'] as const,
}

export async function fetchMobileDevices(
  signal?: AbortSignal,
): Promise<MobileDevice[]> {
  const res = await fetch('/api/v1/mobile/devices', {
    signal,
    cache: 'no-store',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list devices failed: ${res.status}`),
    )
  }
  return (await res.json()) as MobileDevice[]
}

export function mobileDevicesQueryOptions() {
  return queryOptions({
    queryKey: mobileDeviceKeys.all,
    queryFn: ({ signal }) => fetchMobileDevices(signal),
    staleTime: 15_000,
  })
}

export function useMobileDevices() {
  return useQuery(mobileDevicesQueryOptions())
}

// Throws EnvApiError so useStepUpGate can catch step_up_required.
export async function createMobilePairingCode(): Promise<MobilePairingCode> {
  const res = await fetch('/api/v1/mobile/pairing-codes', {
    method: 'POST',
    cache: 'no-store',
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new EnvApiError(
      res.status,
      body?.error ?? `create pairing code failed: ${res.status}`,
      body?.code ?? '',
    )
  }
  return (await res.json()) as MobilePairingCode
}

export function useRevokeMobileDevice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const res = await fetch(
        `/api/v1/mobile/devices/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `revoke device failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: mobileDeviceKeys.all })
      void qc.invalidateQueries({ queryKey: tokenKeys.all })
    },
  })
}
