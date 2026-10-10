import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  ExposurePlan,
  ExposureReport,
  ExposureRestrictInput,
} from '../types/exposure'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { systemDoctorKeys } from './systemDoctor'

const BASE = '/api/v1/firewall/exposure'

export const exposureKeys = {
  all: ['firewall-exposure'] as const,
  report: (probe: boolean) => ['firewall-exposure', { probe }] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  label: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${label} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function useExposure(probe: boolean) {
  return useQuery({
    queryKey: exposureKeys.report(probe),
    queryFn: () =>
      request<ExposureReport>(
        probe ? `${BASE}?probe=true` : BASE,
        undefined,
        'fetch exposure',
      ),
    staleTime: 15_000,
  })
}

function json(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function usePreviewRestriction() {
  return useMutation<ExposurePlan, ApiError, ExposureRestrictInput>({
    mutationFn: (input) =>
      request<ExposurePlan>(
        `${BASE}/preview`,
        json('POST', input),
        'preview restriction',
      ),
  })
}

function useInvalidate() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: exposureKeys.all })
    void queryClient.invalidateQueries({ queryKey: systemDoctorKeys.all })
  }
}

export function useApplyRestriction() {
  const invalidate = useInvalidate()
  return useMutation<ExposurePlan, ApiError, ExposureRestrictInput>({
    mutationFn: (input) =>
      request<ExposurePlan>(
        `${BASE}/restrictions/${input.protocol}/${input.port}`,
        json('PUT', { ...input, confirm: true }),
        'apply restriction',
      ),
    onSuccess: invalidate,
  })
}

export function useRemoveRestriction() {
  const invalidate = useInvalidate()
  return useMutation<void, ApiError, { port: number; protocol: string }>({
    mutationFn: ({ port, protocol }) =>
      request<void>(
        `${BASE}/restrictions/${protocol}/${port}`,
        { method: 'DELETE' },
        'remove restriction',
      ),
    onSuccess: invalidate,
  })
}
