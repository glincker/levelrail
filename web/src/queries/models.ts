// Query keys, fetchers and mutations for /api/v1/models and /api/v1/gpus
// (internal/api/models.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { isModelSettling } from '../lib/models'
import type {
  CreateModelRequest,
  CreateModelResponse,
  GpuNode,
  ModelResource,
} from '../types/models'

export const modelKeys = {
  all: ['models'] as const,
  list: () => [...modelKeys.all, 'list'] as const,
  gpus: () => [...modelKeys.all, 'gpus'] as const,
}

const SETTLING_REFETCH_MS = 3000
const GPU_REFETCH_MS = 30000

export async function requestJson<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export async function requestVoid(
  url: string,
  init: RequestInit,
  what: string,
): Promise<void> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
}

export function fetchModels(): Promise<ModelResource[]> {
  return requestJson<ModelResource[]>(
    '/api/v1/models',
    undefined,
    'fetch models',
  )
}

export function modelListQueryOptions() {
  return queryOptions({
    queryKey: modelKeys.list(),
    queryFn: fetchModels,
    // Poll quickly only while something is downloading, loading or being
    // removed, so an idle page costs nothing.
    refetchInterval: (query) =>
      query.state.data && isModelSettling(query.state.data)
        ? SETTLING_REFETCH_MS
        : false,
  })
}

export function useModels() {
  return useQuery(modelListQueryOptions())
}

export function fetchGpuNodes(): Promise<GpuNode[]> {
  return requestJson<GpuNode[]>('/api/v1/gpus', undefined, 'fetch gpu nodes')
}

export function useGpuNodes() {
  return useQuery({
    queryKey: modelKeys.gpus(),
    queryFn: fetchGpuNodes,
    refetchInterval: GPU_REFETCH_MS,
  })
}

export function useCreateModel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateModelRequest) =>
      requestJson<CreateModelResponse>(
        '/api/v1/models',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'deploy model',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}

function useModelAction(
  what: string,
  build: (name: string) => { url: string; init: RequestInit },
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => {
      const { url, init } = build(name)
      return requestVoid(url, init, what)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}

export function useDeleteModel() {
  return useModelAction('delete model', (name) => ({
    url: `/api/v1/models/${encodeURIComponent(name)}`,
    init: { method: 'DELETE' },
  }))
}

export function useRestartModel() {
  return useModelAction('restart model', (name) => ({
    url: `/api/v1/models/${encodeURIComponent(name)}/restart`,
    init: { method: 'POST' },
  }))
}

export function useRotateModelApiKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      requestJson<{ api_key: string }>(
        `/api/v1/models/${encodeURIComponent(name)}/api-key`,
        { method: 'POST' },
        'rotate api key',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}
