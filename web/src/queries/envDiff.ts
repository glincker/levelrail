import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface EnvDiff {
  app: string
  against: string
  added: string[]
  removed: string[]
  changed: string[]
  same: number
  secret_unchecked: number
}

/** fetchEnvDiff reads GET /api/v1/apps/{name}/env-diff, which returns keys only and never a value. */
export async function fetchEnvDiff(
  app: string,
  against: string,
): Promise<EnvDiff> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(app)}/env-diff?against=${encodeURIComponent(against)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `env diff failed: ${res.status}`),
    )
  }
  return res.json() as Promise<EnvDiff>
}

export function useEnvDiff(app: string, against: string) {
  return useQuery({
    queryKey: ['env-diff', app, against] as const,
    queryFn: () => fetchEnvDiff(app, against),
    enabled: against !== '',
  })
}
