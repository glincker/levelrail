// Fetchers and hooks for the asset bucket endpoints (internal/api/assets*.go, ADR 044).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { directUploadLimit } from '../lib/assetsFormat'
import type {
  AssetBucket,
  AssetList,
  AssetProbe,
  AssetUploadTicket,
  AssetUrls,
  AssetUsage,
  CreateAssetBucketInput,
  UpdateAssetBucketInput,
} from '../types/assets'

export const assetKeys = {
  all: ['assets'] as const,
  buckets: () => [...assetKeys.all, 'buckets'] as const,
  bucket: (id: string) => [...assetKeys.all, 'bucket', id] as const,
  list: (id: string) => [...assetKeys.all, 'list', id] as const,
  usage: (id: string, egress: number) =>
    [...assetKeys.all, 'usage', id, egress] as const,
}

const root = '/api/v1/assets/buckets'
const base = (id: string) => `${root}/${encodeURIComponent(id)}`

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  return (await res.json().catch(() => ({}))) as T
}

function json(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function assetBucketsQueryOptions() {
  return queryOptions({
    queryKey: assetKeys.buckets(),
    queryFn: () => request<AssetBucket[]>(root, undefined, 'list buckets'),
  })
}

export function assetBucketQueryOptions(id: string) {
  return queryOptions({
    queryKey: assetKeys.bucket(id),
    queryFn: () => request<AssetBucket>(base(id), undefined, 'load bucket'),
  })
}

export function assetListQueryOptions(id: string) {
  return queryOptions({
    queryKey: assetKeys.list(id),
    queryFn: () =>
      request<AssetList>(`${base(id)}/assets?limit=100`, undefined, 'list'),
  })
}

export function assetUsageQueryOptions(id: string, egressGb: number) {
  return queryOptions({
    queryKey: assetKeys.usage(id, egressGb),
    queryFn: () =>
      request<AssetUsage>(
        `${base(id)}/usage?egress_gb=${String(egressGb)}`,
        undefined,
        'load usage',
      ),
  })
}

export function useAssetBuckets() {
  return useQuery(assetBucketsQueryOptions())
}

export function useAssetBucket(id: string) {
  return useQuery(assetBucketQueryOptions(id))
}

export function useAssetList(id: string) {
  return useQuery(assetListQueryOptions(id))
}

export function useAssetUsage(id: string, egressGb: number) {
  return useQuery({
    ...assetUsageQueryOptions(id, egressGb),
    placeholderData: (previous) => previous,
  })
}

export function fetchAssetUrls(id: string, key: string): Promise<AssetUrls> {
  return request<AssetUrls>(
    `${base(id)}/urls?key=${encodeURIComponent(key)}`,
    undefined,
    'load urls',
  )
}

const URLS_STALE_MS = 5 * 60 * 1000

export function assetUrlsQueryOptions(id: string, key: string) {
  return queryOptions({
    queryKey: [...assetKeys.all, 'urls', id, key] as const,
    queryFn: () => fetchAssetUrls(id, key),
    staleTime: URLS_STALE_MS,
  })
}

export function useCreateAssetBucket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAssetBucketInput) =>
      request<AssetBucket>(root, json('POST', input), 'connect bucket'),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: assetKeys.buckets() }),
  })
}

export function useUpdateAssetBucket(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: UpdateAssetBucketInput) =>
      request<AssetBucket>(base(id), json('PATCH', input), 'save bucket'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: assetKeys.all }),
  })
}

export function useDeleteAssetBucket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<unknown>(base(id), { method: 'DELETE' }, 'disconnect bucket'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: assetKeys.all }),
  })
}

export function useDeleteAsset(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (key: string) =>
      request<unknown>(
        `${base(id)}/objects?key=${encodeURIComponent(key)}`,
        { method: 'DELETE' },
        'delete asset',
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: assetKeys.all }),
  })
}

export function useProbeAssetBucket(id: string) {
  return useMutation({
    mutationFn: (fixCors: boolean) =>
      request<AssetProbe>(
        `${base(id)}/probe`,
        json('POST', fixCors ? { fix_cors: true } : {}),
        'probe bucket',
      ),
  })
}

export interface UploadRequest {
  bucketId: string
  file: File
  key: string
  onProgress: (fraction: number) => void
}

function putWithProgress(
  method: string,
  url: string,
  headers: Record<string, string>,
  file: File,
  onProgress: (fraction: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open(method, url)
    for (const [name, value] of Object.entries(headers)) {
      xhr.setRequestHeader(name, value)
    }
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && e.total > 0) onProgress(e.loaded / e.total)
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
        return
      }
      reject(new ApiError(xhr.status, `upload failed: ${String(xhr.status)}`))
    }
    xhr.onerror = () => {
      reject(new ApiError(0, 'upload failed: network or CORS error'))
    }
    xhr.send(file)
  })
}

// Small files go through the API (PUT /objects). Larger ones get a signed URL so bytes never cross the control plane.
export async function uploadAsset({
  bucketId,
  file,
  key,
  onProgress,
}: UploadRequest): Promise<void> {
  const contentType = file.type || 'application/octet-stream'
  if (file.size <= directUploadLimit()) {
    await putWithProgress(
      'PUT',
      `${base(bucketId)}/objects?key=${encodeURIComponent(key)}`,
      { 'Content-Type': contentType },
      file,
      onProgress,
    )
    return
  }
  const ticket = await request<AssetUploadTicket>(
    `${base(bucketId)}/uploads`,
    json('POST', { key, content_type: contentType, size: file.size }),
    'start upload',
  )
  await putWithProgress(
    ticket.method,
    ticket.url,
    ticket.headers,
    file,
    onProgress,
  )
  await request<unknown>(
    `${base(bucketId)}/uploads/confirm`,
    json('POST', { key }),
    'confirm upload',
  )
}

export function useUploadAsset() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: uploadAsset,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: assetKeys.all }),
  })
}
