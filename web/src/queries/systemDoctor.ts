// Query-key factory and fetcher for GET /api/v1/system/doctor
// (internal/api's preflight health-check bundle, already shipped as
// levelrail-cli doctor with no dashboard page). Session-cookie
// authenticated like every other settings page. Read-only: no mutations,
// just a suspense query plus a manual refetch for the "Re-run checks"
// button, since each check does a real Docker ping and disk stat call
// and shouldn't be polled automatically.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DoctorCheckStatus = 'ok' | 'warn' | 'fail' | 'unknown'

export interface DoctorCheck {
  code: string
  name: string
  status: DoctorCheckStatus
  message: string
}

export interface DoctorReport {
  ok: boolean
  checks: DoctorCheck[]
}

export const systemDoctorKeys = {
  all: ['system-doctor'] as const,
}

export async function fetchSystemDoctor(): Promise<DoctorReport> {
  const res = await fetch('/api/v1/system/doctor')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch system doctor failed: ${res.status}`),
    )
  }
  return (await res.json()) as DoctorReport
}

export function systemDoctorQueryOptions() {
  return queryOptions({
    queryKey: systemDoctorKeys.all,
    queryFn: fetchSystemDoctor,
  })
}

export function useSystemDoctor() {
  return useSuspenseQuery(systemDoctorQueryOptions())
}
