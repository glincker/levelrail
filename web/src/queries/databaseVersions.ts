import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { DatabaseVersions } from '../types/databaseVersions'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const databaseVersionKeys = {
  engine: (engine: string) => ['database-versions', engine] as const,
}

const STALE_MS = 5 * 60_000

async function request(
  url: string,
  init: RequestInit | undefined,
): Promise<DatabaseVersions> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `database versions failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseVersions
}

export function databaseVersionsQueryOptions(engine: string) {
  return queryOptions({
    queryKey: databaseVersionKeys.engine(engine),
    queryFn: () =>
      request(
        `/api/v1/database-versions?engine=${encodeURIComponent(engine)}`,
        undefined,
      ),
    staleTime: STALE_MS,
    enabled: engine !== '',
  })
}

// Optional by design: a missing catalog must never block the form, the
// picker just falls back to a plain text field.
export function useDatabaseVersions(engine: string) {
  return useQuery({ ...databaseVersionsQueryOptions(engine), retry: false })
}

// Replaces the cached list only on success, so a failed live check keeps
// the previous versions on screen.
export function useCheckDatabaseVersions(engine: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      request(
        `/api/v1/database-versions/refresh?engine=${encodeURIComponent(engine)}`,
        { method: 'POST' },
      ),
    onSuccess: (data) => {
      qc.setQueryData(databaseVersionKeys.engine(engine), data)
    },
  })
}
