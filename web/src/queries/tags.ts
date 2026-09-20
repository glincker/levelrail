// Query-key factory and fetchers for the /tags resource
// (internal/api/tags.go): arbitrary, operator-defined labels for
// organizing and filtering apps. Attach/detach live here too (they're
// app-scoped routes, but their own mutation shape belongs next to the
// tag type they operate on, the same way queries/appVaultEnv.ts sits
// next to its own env-ref type rather than inside queries/apps.ts).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const tagKeys = {
  all: ['tags'] as const,
  list: () => [...tagKeys.all, 'list'] as const,
}

// Tag mirrors internal/api/tags.go's tagResource exactly.
export interface Tag {
  id: string
  name: string
  created_at?: string
}

export async function fetchTags(): Promise<Tag[]> {
  const res = await fetch('/api/v1/tags')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch tags failed: ${res.status}`),
    )
  }
  return (await res.json()) as Tag[]
}

export function tagsQueryOptions() {
  return queryOptions({ queryKey: tagKeys.list(), queryFn: fetchTags })
}

// Non-suspense: both TagsControl (app detail) and TagFilter (apps list)
// render fine with an empty tag list while this is still loading,
// mirroring useAppListOptional's own "degrade, don't block" reasoning.
export function useTags() {
  return useQuery({ ...tagsQueryOptions(), retry: false })
}

// POST /api/v1/apps/{name}/tags (internal/api/tags.go's
// handleAttachAppTag): attaches tagName, creating it first if this is
// the first time it's been used.
export async function attachAppTag(
  appName: string,
  tagName: string,
): Promise<Tag> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: tagName }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `attach tag failed: ${res.status}`),
    )
  }
  return (await res.json()) as Tag
}

export function useAttachAppTag(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (tagName: string) => attachAppTag(appName, tagName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: tagKeys.list() })
    },
  })
}

// DELETE /api/v1/apps/{name}/tags/{id} (handleDetachAppTag): id is the
// tag's ID, not its name (see TagsControl's own doc comment for how a
// chip resolves the name it renders back to an ID).
export async function detachAppTag(
  appName: string,
  tagId: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/tags/${encodeURIComponent(tagId)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `detach tag failed: ${res.status}`),
    )
  }
}

export function useDetachAppTag(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (tagId: string) => detachAppTag(appName, tagId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
