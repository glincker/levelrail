// Query-key factory and fetcher for GET /api/v1/database-engines
// (internal/api/database_engines.go's handleListDatabaseEngines): every
// database engine this control plane can actually create, backed by the
// embedded internal/store/database_engines.yaml registry rather than a
// hardcoded frontend list. CreateDatabaseFields reads this instead of
// hand-maintaining its own postgres/redis/mysql arrays, so a new engine
// added to that registry (plus a real reconciler case) appears here
// automatically, no frontend code change needed for the engine picker
// or its version placeholder.

import { useQuery, queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const databaseEnginesKeys = {
  all: ['database-engines'] as const,
}

// DatabaseEngineInfo mirrors internal/api/database_engines.go's
// databaseEngineResource wire shape exactly.
export interface DatabaseEngineInfo {
  id: string
  label: string
  default_version: string
}

export async function fetchDatabaseEngines(): Promise<DatabaseEngineInfo[]> {
  const res = await fetch('/api/v1/database-engines')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch database engines failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DatabaseEngineInfo[]
}

// staleTime matches certificatesQueryOptions's own reasoning: this list
// changes only when the control plane itself is upgraded with a new
// registry entry, not during a normal session, so there's no reason to
// refetch it aggressively.
export function databaseEnginesQueryOptions() {
  return queryOptions({
    queryKey: databaseEnginesKeys.all,
    queryFn: fetchDatabaseEngines,
    staleTime: 60_000,
  })
}

// Not suspense: this call backs a dialog's own internal Select and
// version-placeholder text, not a route's primary content, the same
// "must never block this form's core function" reasoning
// useNodeListOptional's own doc comment (queries/nodes.ts) already
// establishes for a different optional-convenience list. A failure or
// slow load here just means an empty Select/placeholder momentarily,
// never a blocked dialog.
export function useDatabaseEnginesOptional() {
  return useQuery({ ...databaseEnginesQueryOptions(), retry: false })
}
