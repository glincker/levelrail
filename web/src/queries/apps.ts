// Query-key factory and fetchers for the /apps resource, per
// frontend-plan.md section 2 and 4: no ad hoc key arrays inline in
// components, so invalidation after a deploy/rollback action can target
// the right keys precisely. Every route that needs app data imports these
// instead of writing its own fetch call or query key.

import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { AppListFilters, AppPage } from '../types/app'
import type { AppDetail } from '../types/appDetail'

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

// Fetches the full app resource for the detail route, GET
// /api/v1/apps/{name} (internal/api/apps.go's handleGetApp). Distinct
// from fetchAppPage above: that returns list-page summary rows, this
// returns everything the detail route needs to read and, on save,
// re-send in full (PUT is a full replace, see updateApp below).
export async function fetchApp(name: string): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}`)
  if (res.status === 404) {
    throw new Error(`app not found: ${name}`)
  }
  if (!res.ok) {
    throw new Error(`fetch app failed: ${res.status}`)
  }
  return (await res.json()) as AppDetail
}

// Same shared-options pattern appListQueryOptions already established:
// one definition both the route loader's queryClient.ensureQueryData
// call and the component's useSuspenseQuery/useApp call share, so the
// AppDetail type flows through without either call site re-declaring it.
export function appDetailQueryOptions(name: string) {
  return queryOptions({
    queryKey: appKeys.detail(name),
    queryFn: () => fetchApp(name),
  })
}

// Thin wrapper the detail route's component uses instead of calling
// useSuspenseQuery(appDetailQueryOptions(name)) directly, mirroring the
// useApp(name) shape named in the frontend-plan style already
// established by appListQueryOptions/fetchAppPage above.
export function useApp(name: string) {
  return useSuspenseQuery(appDetailQueryOptions(name))
}

async function readErrorMessage(
  res: Response,
  fallback: string,
): Promise<string> {
  const body = (await res.json().catch(() => null)) as { error?: string } | null
  return body?.error ?? fallback
}

// PUT /api/v1/apps/{name} (internal/api/apps.go's handleUpdateApp), full
// replace semantics: the whole AppDetail is sent back on every call,
// there is no partial-patch endpoint. Callers (DomainEditor, EnvEditor)
// are responsible for spreading the current app and only swapping the
// field they actually edited.
export async function updateApp(app: AppDetail): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(app.name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(app),
  })
  if (!res.ok) {
    throw new Error(
      await readErrorMessage(res, `update app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

// Shared mutation hook for both DomainEditor and EnvEditor: same
// endpoint, same full-replace payload shape, only the field that
// changed differs between call sites. On success, writes the server's
// response straight into the detail query's cache rather than
// invalidating and refetching, since the PUT response already is the
// new canonical AppDetail.
export function useUpdateApp(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateApp,
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(name), updated)
    },
  })
}
