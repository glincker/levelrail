// Read-only verifier of a domain that is moving here (internal/api/domain_cutover_verify.go).
import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type CutoverVerifyVerdict = 'PASS' | 'WARN' | 'FAIL'

export interface CutoverVerifyReport {
  domain: string
  verdict: CutoverVerifyVerdict
  summary: string
  checked_at: string
}

export const domainCutoverVerifyKeys = {
  one: (domain: string) => ['domains', domain, 'cutover-verify'] as const,
}

// The check makes real requests to the public name, so it is not polled.
export function domainCutoverVerifyQueryOptions(domain: string) {
  return queryOptions({
    queryKey: domainCutoverVerifyKeys.one(domain),
    queryFn: async (): Promise<CutoverVerifyReport> => {
      const res = await fetch(
        `/api/v1/domains/${encodeURIComponent(domain)}/verify`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `verify failed: ${res.status}`),
        )
      }
      return (await res.json()) as CutoverVerifyReport
    },
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })
}
