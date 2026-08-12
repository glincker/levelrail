// Query-key factory and fetchers for the /apps resource, per
// frontend-plan.md section 2 and 4: no ad hoc key arrays inline in
// components, so invalidation after a deploy/rollback action can target
// the right keys precisely. Every route that needs app data imports these
// instead of writing its own fetch call or query key.

import { infiniteQueryOptions } from '@tanstack/react-query'
import type { AppListFilters, AppPage } from '../types/app'

export const appKeys = {
  all: ['apps'] as const,
  list: (filters?: AppListFilters) =>
    [...appKeys.all, 'list', filters ?? {}] as const,
  detail: (appId: string) => [...appKeys.all, 'detail', appId] as const,
}

interface FetchAppPageArgs {
  pageParam: string | null
}

// Fetches one cursor page of app summaries from the control plane API.
// Per CLAUDE.md 4.9/4.11 the HTTP API is versioned under /api/v1; this is
// deliberately a thin fetch wrapper with no client-side caching of its
// own, TanStack Query owns caching via appKeys above. It is fine for this
// to 404/fail at runtime until internal/api's apps endpoint exists: the
// point of this pass is that the frontend code is correct and buildable.
export async function fetchAppPage({
  pageParam,
}: FetchAppPageArgs): Promise<AppPage> {
  const url = new URL('/api/v1/apps', window.location.origin)
  if (pageParam) {
    url.searchParams.set('cursor', pageParam)
  }

  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(`fetch apps failed: ${res.status}`)
  }

  return (await res.json()) as AppPage
}

// Shared options object between the route loader's
// queryClient.ensureInfiniteQueryData call and the component's
// useSuspenseInfiniteQuery call. Defining it once, per the TanStack Query
// docs' recommended pattern, is what lets TypeScript carry the AppPage
// type through to lastPage in getNextPageParam without either call site
// re-declaring generics by hand.
export function appListQueryOptions(filters?: AppListFilters) {
  return infiniteQueryOptions({
    queryKey: appKeys.list(filters),
    queryFn: fetchAppPage,
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage: AppPage) => lastPage.nextCursor,
  })
}
