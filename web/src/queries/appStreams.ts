// Query-key factory and fetchers for /api/v1/apps/{name}/streams,
// mirroring queries/firewallRules.ts's own shared-queryOptions pattern,
// scoped per app name since a stream always belongs to exactly one app.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { AppStream, CreateAppStreamRequest } from '../types/appStream'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const appStreamKeys = {
  all: ['app-streams'] as const,
  list: (name: string) => [...appStreamKeys.all, name] as const,
}

export async function fetchAppStreams(name: string): Promise<AppStream[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/streams`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app streams failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppStream[]
}

export function appStreamListQueryOptions(name: string) {
  return queryOptions({
    queryKey: appStreamKeys.list(name),
    queryFn: () => fetchAppStreams(name),
  })
}

export function useAppStreams(name: string) {
  return useSuspenseQuery(appStreamListQueryOptions(name))
}

export async function createAppStream(
  name: string,
  req: CreateAppStreamRequest,
): Promise<AppStream> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/streams`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create app stream failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppStream
}

export function useCreateAppStream(name: string) {
  const queryClient = useQueryClient()
  return useMutation<AppStream, ApiError, CreateAppStreamRequest>({
    mutationFn: (req) => createAppStream(name, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appStreamKeys.list(name) })
    },
  })
}

export async function deleteAppStream(name: string, id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/streams/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `delete app stream failed: ${res.status}`),
  )
}

export function useDeleteAppStream(name: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => deleteAppStream(name, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appStreamKeys.list(name) })
    },
  })
}
