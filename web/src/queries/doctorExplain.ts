import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/fixit's Explanation.
export interface DoctorExplanation {
  id: string
  plain: string
  why: string
  next: string
  fix_id?: string
  kind: string
}

export const doctorExplainKeys = {
  one: (id: string) => ['doctor-explain', id] as const,
}

export function doctorExplainQueryOptions(id: string) {
  return queryOptions({
    queryKey: doctorExplainKeys.one(id),
    queryFn: async (): Promise<DoctorExplanation> => {
      const res = await fetch(
        `/api/v1/doctor/explain?id=${encodeURIComponent(id)}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `explain check failed: ${res.status}`),
        )
      }
      return (await res.json()) as DoctorExplanation
    },
    // The server text is a fixed table, so it never goes stale in a session.
    staleTime: Infinity,
  })
}

/** Lazy: nothing is fetched until `enabled` flips true (first open). */
export function useDoctorExplanation(id: string, enabled: boolean) {
  return useQuery({ ...doctorExplainQueryOptions(id), enabled })
}
