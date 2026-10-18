import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ImageRole = 'live' | 'rollback_target' | 'recent' | 'history'

export interface ImageAppUse {
  app: string
  role: ImageRole
  deploy_id?: string
  source?: string
  commit_sha?: string
  branch?: string
  at?: string
}

// Matches internal/imagelife's Entry.
export interface ImageEntry {
  id: string
  digest?: string
  tags: string[]
  size_bytes: number
  shared_bytes: number
  containers: number
  created_at: string
  apps: ImageAppUse[]
  protected: boolean
  protect_reason?: string
}

export interface ImageInventory {
  node: string
  images: ImageEntry[]
  totals: {
    count: number
    size_bytes: number
    reclaimable_bytes: number
    protected_count: number
  }
  retention: { keep_per_app: number; min_age_hours: number }
  signatures: { mode: 'off' | 'enforce'; configured: boolean }
}

export interface ImageLayer {
  id: string
  created_by: string
  size_bytes: number
  created_at: string
}

export interface ImageLayers {
  layers: ImageLayer[]
  size_bytes: number
}

export interface ImageGCResult {
  dry_run: boolean
  candidates: ImageEntry[]
  removed: string[]
  failed: { id: string; error: string }[]
  reclaimed_bytes: number
}

export type ImageVerifyStatus =
  'verified' | 'unsigned' | 'failed' | 'not_configured' | 'no_registry_digest'

export interface ImageVerifyResult {
  status: ImageVerifyStatus
  message: string
}

export const imageKeys = {
  all: ['image-inventory'] as const,
  list: (node: string) => ['image-inventory', 'list', node] as const,
  layers: (node: string, id: string) =>
    ['image-inventory', 'layers', node, id] as const,
}

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function imageInventoryQueryOptions(node: string) {
  return queryOptions({
    queryKey: imageKeys.list(node),
    queryFn: async ({ signal }) =>
      json<ImageInventory>(
        await fetch(`/api/v1/images?node=${encodeURIComponent(node)}`, {
          signal,
        }),
        'fetch images',
      ),
  })
}

export function useImageInventory(node: string) {
  return useQuery(imageInventoryQueryOptions(node))
}

export function useImageLayers(node: string, id: string | null) {
  return useQuery({
    queryKey: imageKeys.layers(node, id ?? ''),
    enabled: id !== null,
    queryFn: async ({ signal }) =>
      json<ImageLayers>(
        await fetch(
          `/api/v1/images/layers?node=${encodeURIComponent(node)}&id=${encodeURIComponent(id ?? '')}`,
          { signal },
        ),
        'fetch image layers',
      ),
  })
}

export function useImageGC(node: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (dryRun: boolean): Promise<ImageGCResult> =>
      json<ImageGCResult>(
        await fetch('/api/v1/images/gc', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ node, dry_run: dryRun }),
        }),
        'image cleanup',
      ),
    onSuccess: (res) => {
      if (!res.dry_run) void qc.invalidateQueries({ queryKey: imageKeys.all })
    },
  })
}

export function useVerifyImage(node: string) {
  return useMutation({
    mutationFn: async (id: string): Promise<ImageVerifyResult> =>
      json<ImageVerifyResult>(
        await fetch('/api/v1/images/verify', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ node, id }),
        }),
        'verify image',
      ),
  })
}
