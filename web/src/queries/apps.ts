// Query-key factory and fetchers for the /apps resource, per
// frontend-plan.md section 2 and 4: no ad hoc key arrays inline in
// components, so invalidation after a deploy/rollback action can target
// the right keys precisely. Every route that needs app data imports these
// instead of writing its own fetch call or query key.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const appKeys = {
  all: ['apps'] as const,
  list: () => [...appKeys.all, 'list'] as const,
  detail: (appId: string) => [...appKeys.all, 'detail', appId] as const,
}

// Fetches every app from the control plane API. GET /api/v1/apps
// (internal/api/apps.go's handleListApps) returns a bare array of the
// same appResource shape the detail endpoint returns, no cursor and no
// separate summary projection, so this is the one list fetcher, not a
// page-at-a-time one: there's nothing server-side yet to paginate
// against.
export async function fetchApps(): Promise<AppDetail[]> {
  const res = await fetch('/api/v1/apps')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch apps failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail[]
}

// Shared options object between the route loader's
// queryClient.ensureQueryData call and the component's useSuspenseQuery
// call, the same pattern appDetailQueryOptions below uses.
export function appListQueryOptions() {
  return queryOptions({
    queryKey: appKeys.list(),
    queryFn: fetchApps,
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
    throw new ApiError(404, `app not found: ${name}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app failed: ${res.status}`),
    )
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
    throw new ApiError(
      res.status,
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
      // The list route renders each app's domains too (AppRow), so an
      // edit here must not leave a stale copy sitting in that cache.
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
