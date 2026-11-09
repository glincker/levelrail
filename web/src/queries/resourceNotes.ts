import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type NotesKindPath = 'apps' | 'databases'

export interface ResourceNotes {
  description: string
  notes: string
  updated_by?: string
  updated_at?: string
}

export const resourceNotesKeys = {
  one: (path: NotesKindPath, name: string) =>
    ['resource-notes', path, name] as const,
}

function notesUrl(path: NotesKindPath, name: string): string {
  return `/api/v1/${path}/${encodeURIComponent(name)}/notes`
}

export function useResourceNotes(path: NotesKindPath, name: string) {
  return useQuery({
    queryKey: resourceNotesKeys.one(path, name),
    retry: false,
    queryFn: async (): Promise<ResourceNotes> => {
      const res = await fetch(notesUrl(path, name))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch notes failed: ${String(res.status)}`,
          ),
        )
      }
      return (await res.json()) as ResourceNotes
    },
  })
}

export function useSetResourceNotes(path: NotesKindPath, name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: async (body: { description: string; notes: string }) => {
      const res = await fetch(notesUrl(path, name), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `save notes failed: ${String(res.status)}`,
          ),
        )
      }
      return (await res.json()) as ResourceNotes
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(resourceNotesKeys.one(path, name), saved)
      void queryClient.invalidateQueries({ queryKey: ['apps'] })
    },
  })
}
