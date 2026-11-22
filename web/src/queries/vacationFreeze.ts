// GET/PUT/DELETE /api/v1/settings/vacation-freeze (internal/api/vacation_freeze.go):
// one switch that holds auto deploys and scheduled jobs for N days.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { useApiMutation } from './useApiMutation'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '@/lib/pollTier'

export interface VacationFreeze {
  active: boolean
  until?: string
  days_left: number
  reason?: string
}

const ENDPOINT = '/api/v1/settings/vacation-freeze'

export const vacationFreezeKeys = {
  all: ['settings', 'vacation-freeze'] as const,
}

async function call(
  method: 'GET' | 'PUT' | 'DELETE',
  body?: { days: number; reason?: string },
): Promise<VacationFreeze> {
  const res = await fetch(ENDPOINT, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `vacation freeze failed: ${res.status}`),
    )
  }
  return (await res.json()) as VacationFreeze
}

export function vacationFreezeQueryOptions() {
  return queryOptions({
    queryKey: vacationFreezeKeys.all,
    queryFn: () => call('GET'),
    // The freeze lifts itself, so the banner must notice without a reload.
    refetchInterval: pollMs('idle'),
    retry: false,
  })
}

export function useVacationFreeze() {
  return useQuery(vacationFreezeQueryOptions())
}

export function useSetVacationFreeze() {
  return useApiMutation<
    VacationFreeze,
    { days: number; reason?: string } | null
  >({
    fn: (req) => (req ? call('PUT', req) : call('DELETE')),
    invalidate: [vacationFreezeKeys.all],
  })
}
