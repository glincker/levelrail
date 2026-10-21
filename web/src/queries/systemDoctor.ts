// Doctor report types and fetchers: GET /api/v1/system/doctor for the
// one-shot report, the per-check re-run, and the stream event shape.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { DEDUPE_STALE_MS } from '../lib/queryStale'

export type DoctorCheckStatus = 'ok' | 'warn' | 'fail' | 'unknown'

export interface DoctorCheck {
  code: string
  name: string
  status: DoctorCheckStatus
  message: string
  /** A copy-pasteable shell command that resolves this check, when there is one. */
  fix?: string
  /** Path relative to this instance's docs root (brand.DocsURL), paired with fix. */
  docs_path?: string
  /** Set on a check that is not ok: a broken thing or hardening advice. */
  class?: 'incident' | 'recommendation'
  /** Section the server filed the check under. */
  group?: string
  /** The registered check that produced this result: the id to re-run it by. */
  check_id?: string
  /** The raw fact behind the message: a path, a port holder, a count. */
  evidence?: string
  /** Typed fix for a failing check, when the server has one. */
  fix_id?: string
  fix_kind?: 'auto' | 'guided' | 'manual'
}

/** One server-sent message from /api/v1/system/doctor/stream. */
export interface DoctorStreamEvent {
  type: 'start' | 'check' | 'fixes' | 'done'
  id?: string
  checks?: DoctorCheck[]
  ok?: boolean
  total?: number
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
    staleTime: DEDUPE_STALE_MS,
  })
}

/** runDoctorCheck re-runs one registered check, the Re-check button's call. */
export async function runDoctorCheck(id: string): Promise<DoctorCheck[]> {
  const res = await fetch(
    `/api/v1/system/doctor/checks/${encodeURIComponent(id)}/run`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `re-run check failed: ${res.status}`),
    )
  }
  return ((await res.json()) as { checks: DoctorCheck[] }).checks
}

export function useSystemDoctor() {
  return useSuspenseQuery(systemDoctorQueryOptions())
}
