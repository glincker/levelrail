// Query-key factory, fetchers and mutations for storage destinations
// (internal/api/storage_destinations.go) and log archive
// (internal/api/log_archive.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CreateStorageDestinationRequest,
  LogArchiveDumpRequest,
  LogArchiveObjectsPage,
  LogArchivePolicy,
  LogArchiveRun,
  SetLogArchivePolicyRequest,
  StorageDestination,
  StorageProbeResult,
  StorageProvider,
} from '../types/storage'

export const storageKeys = {
  all: ['storage'] as const,
  providers: () => [...storageKeys.all, 'providers'] as const,
  destinations: () => [...storageKeys.all, 'destinations'] as const,
  policies: () => [...storageKeys.all, 'archive-policies'] as const,
  runs: (app: string) => [...storageKeys.all, 'archive-runs', app] as const,
  objects: (target: string, app: string) =>
    [...storageKeys.all, 'archive-objects', target, app] as const,
}

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
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

export function storageProvidersQueryOptions() {
  return queryOptions({
    queryKey: storageKeys.providers(),
    queryFn: () =>
      requestJson<StorageProvider[]>(
        '/api/v1/storage/providers',
        undefined,
        'fetch storage providers',
      ),
    staleTime: Infinity,
  })
}

export function storageDestinationsQueryOptions() {
  return queryOptions({
    queryKey: storageKeys.destinations(),
    queryFn: () =>
      requestJson<StorageDestination[]>(
        '/api/v1/storage/destinations',
        undefined,
        'fetch storage destinations',
      ),
  })
}

export function useStorageProviders() {
  return useSuspenseQuery(storageProvidersQueryOptions())
}

export function useStorageDestinations() {
  return useSuspenseQuery(storageDestinationsQueryOptions())
}

// Non-suspending: the app Logs page must still render when storage is not
// configured (501) or the list fails.
export function useStorageDestinationsOptional() {
  return useQuery({ ...storageDestinationsQueryOptions(), retry: false })
}

export function useCreateStorageDestination() {
  const queryClient = useQueryClient()
  return useMutation<
    StorageDestination,
    ApiError,
    CreateStorageDestinationRequest
  >({
    mutationFn: (req) =>
      requestJson<StorageDestination>(
        '/api/v1/storage/destinations',
        jsonInit('POST', req),
        'create storage destination',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: storageKeys.destinations() }),
  })
}

export function useDeleteStorageDestination() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      const res = await fetch(
        `/api/v1/storage/destinations/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
      )
      if (res.status !== 204) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `delete failed: ${res.status}`),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: storageKeys.destinations() }),
  })
}

export function useTestStorageDestination() {
  return useMutation<StorageProbeResult, ApiError, string>({
    mutationFn: (id) =>
      requestJson<StorageProbeResult>(
        `/api/v1/storage/destinations/${encodeURIComponent(id)}/test`,
        jsonInit('POST'),
        'test storage destination',
      ),
  })
}

export function logArchivePoliciesQueryOptions() {
  return queryOptions({
    queryKey: storageKeys.policies(),
    queryFn: () =>
      requestJson<LogArchivePolicy[]>(
        '/api/v1/log-archive/policies',
        undefined,
        'fetch log archive policies',
      ),
    retry: false,
  })
}

export function useLogArchivePolicies() {
  return useQuery(logArchivePoliciesQueryOptions())
}

export function useSetLogArchivePolicy() {
  const queryClient = useQueryClient()
  return useMutation<LogArchivePolicy, ApiError, SetLogArchivePolicyRequest>({
    mutationFn: (req) =>
      requestJson<LogArchivePolicy>(
        '/api/v1/log-archive/policy',
        jsonInit('PUT', req),
        'save log archive policy',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: storageKeys.policies() })
      void queryClient.invalidateQueries({
        queryKey: storageKeys.destinations(),
      })
    },
  })
}

export function useDeleteLogArchivePolicy() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (app) => {
      const res = await fetch(
        `/api/v1/log-archive/policy?app=${encodeURIComponent(app)}`,
        { method: 'DELETE' },
      )
      if (res.status !== 204) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `remove policy failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: storageKeys.policies() })
      void queryClient.invalidateQueries({
        queryKey: storageKeys.destinations(),
      })
    },
  })
}

// Polls while any run is still running so a manual dump's outcome appears
// without a manual refresh.
export function useLogArchiveRuns(app: string) {
  return useQuery({
    queryKey: storageKeys.runs(app),
    queryFn: () =>
      requestJson<LogArchiveRun[]>(
        `/api/v1/log-archive/runs?app=${encodeURIComponent(app)}${app === '' ? '&all=true' : ''}`,
        undefined,
        'fetch log archive runs',
      ),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((r) => r.status === 'running') ? 3000 : false,
  })
}

export function useStartLogArchiveDump(app: string) {
  const queryClient = useQueryClient()
  return useMutation<LogArchiveRun, ApiError, LogArchiveDumpRequest>({
    mutationFn: (req) =>
      requestJson<LogArchiveRun>(
        '/api/v1/log-archive/dump',
        jsonInit('POST', req),
        'start log dump',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: storageKeys.runs(app) }),
  })
}

export function useLogArchiveObjects(target: string, app: string) {
  return useQuery({
    queryKey: storageKeys.objects(target, app),
    queryFn: () =>
      requestJson<LogArchiveObjectsPage>(
        `/api/v1/log-archive/objects?target_id=${encodeURIComponent(target)}${app === '' ? '' : `&app=${encodeURIComponent(app)}`}`,
        undefined,
        'list archived logs',
      ),
    enabled: target !== '',
    retry: false,
  })
}

export function logArchiveDownloadUrl(target: string, key: string): string {
  return `/api/v1/log-archive/objects/download?target_id=${encodeURIComponent(target)}&key=${encodeURIComponent(key)}`
}
