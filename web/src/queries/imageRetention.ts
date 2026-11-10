import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/api's imageRetentionResource.
export interface ImageRetention {
  app: string
  keep: number
  effective_keep: number
  source: 'app' | 'app_default' | 'global'
  global_keep: number
  min_keep: number
  protected_environment?: string
}

export interface ImageRetentionPreview {
  app: string
  keep: number
  effective_keep: number
  dry_run: boolean
  remove: {
    id: string
    tags: string[]
    size_bytes: number
    reclaim_bytes: number
    created_at: string
  }[]
  remove_count: number
  reclaim_bytes: number
  kept: { id: string; tags: string[]; reason: string }[]
}

const key = (app: string) => ['image-retention', app] as const
const path = (app: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/image-retention`

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useImageRetention(app: string) {
  return useQuery({
    queryKey: key(app),
    queryFn: async ({ signal }) =>
      json<ImageRetention>(
        await fetch(path(app), { signal }),
        'fetch image retention',
      ),
  })
}

export function usePreviewImageRetention(app: string) {
  return useMutation({
    mutationFn: async (keep: number) =>
      json<ImageRetentionPreview>(
        await fetch(`${path(app)}/preview`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ keep }),
        }),
        'preview image retention',
      ),
  })
}

export function useSetImageRetention(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (keep: number) =>
      json<ImageRetention>(
        await fetch(path(app), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ keep }),
        }),
        'save image retention',
      ),
    onSuccess: (data) => {
      qc.setQueryData(key(app), data)
      void qc.invalidateQueries({ queryKey: ['image-inventory'] })
    },
  })
}
