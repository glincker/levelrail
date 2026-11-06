// GET /api/v1/status-reasons (internal/api/status_reasons.go): the plain
// explanation and next step for every deploy status reason.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface StatusReason {
  code: string
  kind: string
  meaning: string
  next_step: string
}

export const statusReasonKeys = {
  all: ['status-reasons'] as const,
}

export async function fetchStatusReasons(): Promise<StatusReason[]> {
  const res = await fetch('/api/v1/status-reasons')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch status reasons failed: ${res.status}`),
    )
  }
  return (await res.json()) as StatusReason[]
}

// The catalog only changes with a release, so it is fetched once per session.
export function statusReasonsQueryOptions() {
  return queryOptions({
    queryKey: statusReasonKeys.all,
    queryFn: fetchStatusReasons,
    staleTime: Infinity,
    retry: false,
  })
}

export function useStatusReasons() {
  return useQuery(statusReasonsQueryOptions())
}
