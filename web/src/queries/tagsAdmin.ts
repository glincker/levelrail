// Tag manager calls (create, rename, delete, apps per tag). Kept out of
// queries/tags.ts so the main chunk, which imports that file, stays small.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { tagKeys, type Tag } from './tags'

export interface TagApp {
  name: string
}

async function send(
  url: string,
  method: string,
  what: string,
  body?: unknown,
): Promise<Response> {
  const res = await fetch(
    url,
    body === undefined
      ? { method }
      : {
          method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return res
}

export function tagAppsQueryOptions(tagId: string) {
  return queryOptions({
    queryKey: [...tagKeys.all, 'apps', tagId] as const,
    queryFn: async (): Promise<TagApp[]> => {
      const res = await send(
        `/api/v1/tags/${encodeURIComponent(tagId)}/apps`,
        'GET',
        'fetch tag apps',
      )
      return (await res.json()) as TagApp[]
    },
  })
}

export function useTagApps(tagId: string, enabled: boolean) {
  return useQuery({ ...tagAppsQueryOptions(tagId), enabled, retry: false })
}

function useTagMutation<V>(fn: (v: V) => Promise<unknown>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tagKeys.all }),
  })
}

export function useCreateTag() {
  return useTagMutation(async (name: string) => {
    const res = await send('/api/v1/tags', 'POST', 'create tag', { name })
    return (await res.json()) as Tag
  })
}

export function useRenameTag() {
  return useTagMutation(async (v: { id: string; name: string }) => {
    const res = await send(
      `/api/v1/tags/${encodeURIComponent(v.id)}`,
      'PATCH',
      'rename tag',
      { name: v.name },
    )
    return (await res.json()) as Tag
  })
}

export function useDeleteTag() {
  return useTagMutation(async (id: string) => {
    await send(`/api/v1/tags/${encodeURIComponent(id)}`, 'DELETE', 'delete tag')
  })
}
