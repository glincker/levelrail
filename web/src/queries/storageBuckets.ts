// Query-key factory, fetchers and mutations for named storage buckets
// (internal/api/storage_buckets.go).

import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  BucketObjectsPage,
  BucketPresign,
  BucketSweepResult,
  BucketUsage,
  CreateStorageBucketRequest,
  StorageBucket,
  UpdateStorageBucketRequest,
} from '../types/storageBuckets'

export const bucketKeys = {
  all: ['storage-buckets'] as const,
  list: (app: string) => [...bucketKeys.all, 'list', app] as const,
  usage: (id: string) => [...bucketKeys.all, 'usage', id] as const,
  objects: (id: string, prefix: string) =>
    [...bucketKeys.all, 'objects', id, prefix] as const,
}

const bucketUrl = (id: string) =>
  `/api/v1/storage/buckets/${encodeURIComponent(id)}`

async function requestJson<T>(
  input: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(input, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function useStorageBuckets(app: string) {
  return useQuery({
    queryKey: bucketKeys.list(app),
    queryFn: () =>
      requestJson<StorageBucket[]>(
        `/api/v1/storage/buckets?app=${encodeURIComponent(app)}`,
        undefined,
        'fetch buckets',
      ),
    retry: false,
  })
}

export function useAllStorageBuckets() {
  return useQuery({
    queryKey: bucketKeys.list(''),
    queryFn: () =>
      requestJson<StorageBucket[]>(
        '/api/v1/storage/buckets',
        undefined,
        'fetch buckets',
      ),
    retry: false,
  })
}

export function useBucketUsage(id: string) {
  return useQuery({
    queryKey: bucketKeys.usage(id),
    queryFn: () =>
      requestJson<BucketUsage>(
        `${bucketUrl(id)}/usage`,
        undefined,
        'fetch bucket usage',
      ),
    retry: false,
  })
}

export function useBucketObjects(id: string, prefix: string) {
  return useInfiniteQuery({
    queryKey: bucketKeys.objects(id, prefix),
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      requestJson<BucketObjectsPage>(
        `${bucketUrl(id)}/objects?prefix=${encodeURIComponent(prefix)}&token=${encodeURIComponent(pageParam)}`,
        undefined,
        'list files',
      ),
    getNextPageParam: (last) => last.next || undefined,
    placeholderData: keepPreviousData,
    retry: false,
  })
}

export function bucketDownloadUrl(id: string, key: string): string {
  return `${bucketUrl(id)}/objects/download?key=${encodeURIComponent(key)}`
}

function useInvalidateBuckets() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: bucketKeys.all })
}

export function useCreateStorageBucket() {
  const invalidate = useInvalidateBuckets()
  return useMutation<StorageBucket, ApiError, CreateStorageBucketRequest>({
    mutationFn: (req) =>
      requestJson<StorageBucket>(
        '/api/v1/storage/buckets',
        jsonInit('POST', req),
        'create bucket',
      ),
    onSuccess: invalidate,
  })
}

export function useUpdateStorageBucket() {
  const invalidate = useInvalidateBuckets()
  return useMutation<
    StorageBucket,
    ApiError,
    { id: string; req: UpdateStorageBucketRequest }
  >({
    mutationFn: ({ id, req }) =>
      requestJson<StorageBucket>(
        bucketUrl(id),
        jsonInit('PATCH', req),
        'update bucket',
      ),
    onSuccess: invalidate,
  })
}

export function useDeleteStorageBucket() {
  const invalidate = useInvalidateBuckets()
  return useMutation<void, ApiError, { id: string; purge: boolean }>({
    mutationFn: ({ id, purge }) =>
      requestJson<void>(
        `${bucketUrl(id)}${purge ? '?purge=true' : ''}`,
        { method: 'DELETE' },
        'delete bucket',
      ),
    onSuccess: invalidate,
  })
}

export function useBindAppBucket() {
  const invalidate = useInvalidateBuckets()
  return useMutation<
    StorageBucket,
    ApiError,
    { app: string; id: string; envPrefix?: string }
  >({
    mutationFn: ({ app, id, envPrefix }) =>
      requestJson<StorageBucket>(
        `/api/v1/apps/${encodeURIComponent(app)}/buckets/${encodeURIComponent(id)}`,
        jsonInit('PUT', { env_prefix: envPrefix ?? '' }),
        'bind bucket',
      ),
    onSuccess: invalidate,
  })
}

export function useUnbindAppBucket() {
  const invalidate = useInvalidateBuckets()
  return useMutation<
    { app_restarted: boolean },
    ApiError,
    { app: string; id: string }
  >({
    mutationFn: ({ app, id }) =>
      requestJson<{ app_restarted: boolean }>(
        `/api/v1/apps/${encodeURIComponent(app)}/buckets/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'unbind bucket',
      ),
    onSuccess: invalidate,
  })
}

export function useUploadBucketObject() {
  const invalidate = useInvalidateBuckets()
  return useMutation<
    unknown,
    ApiError,
    { id: string; key: string; file: File }
  >({
    mutationFn: ({ id, key, file }) =>
      requestJson<unknown>(
        `${bucketUrl(id)}/objects?key=${encodeURIComponent(key)}`,
        {
          method: 'PUT',
          headers: file.type ? { 'Content-Type': file.type } : undefined,
          body: file,
        },
        'upload file',
      ),
    onSuccess: invalidate,
  })
}

export function useDeleteBucketObject() {
  const invalidate = useInvalidateBuckets()
  return useMutation<void, ApiError, { id: string; key: string }>({
    mutationFn: ({ id, key }) =>
      requestJson<void>(
        `${bucketUrl(id)}/objects?key=${encodeURIComponent(key)}`,
        { method: 'DELETE' },
        'delete file',
      ),
    onSuccess: invalidate,
  })
}

export function usePresignBucketObject() {
  return useMutation<
    BucketPresign,
    ApiError,
    { id: string; key: string; method: 'GET' | 'PUT'; expiresSeconds: number }
  >({
    mutationFn: ({ id, key, method, expiresSeconds }) =>
      requestJson<BucketPresign>(
        `${bucketUrl(id)}/presign`,
        jsonInit('POST', { key, method, expires_seconds: expiresSeconds }),
        'presign file',
      ),
  })
}

export function useRunBucketLifecycle() {
  const invalidate = useInvalidateBuckets()
  return useMutation<BucketSweepResult, ApiError, string>({
    mutationFn: (id) =>
      requestJson<BucketSweepResult>(
        `${bucketUrl(id)}/lifecycle/run`,
        { method: 'POST' },
        'run lifecycle',
      ),
    onSuccess: invalidate,
  })
}
