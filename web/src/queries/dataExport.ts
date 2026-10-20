// Query keys, fetchers and mutations for /api/v1/export/destinations
// (internal/api/data_export.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CreateDataExportDestinationRequest,
  DataExportDestination,
  DataExportTestResult,
  UpdateDataExportDestinationRequest,
} from '../types/dataExport'

export const dataExportKeys = {
  all: ['data-export'] as const,
  destinations: () => [...dataExportKeys.all, 'destinations'] as const,
}

// Delivery counters change every few seconds, so the list is polled.
const DESTINATIONS_REFETCH_MS = 10_000

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
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
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

export function dataExportDestinationsQueryOptions() {
  return queryOptions({
    queryKey: dataExportKeys.destinations(),
    queryFn: () =>
      request<DataExportDestination[]>(
        '/api/v1/export/destinations',
        undefined,
        'fetch export destinations',
      ),
    refetchInterval: DESTINATIONS_REFETCH_MS,
    retry: false,
  })
}

export function useDataExportDestinations() {
  return useQuery(dataExportDestinationsQueryOptions())
}

function useInvalidatingMutation<TVars, TData>(
  fn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: dataExportKeys.destinations(),
      }),
  })
}

export function useCreateDataExportDestination() {
  return useInvalidatingMutation((req: CreateDataExportDestinationRequest) =>
    request<DataExportDestination>(
      '/api/v1/export/destinations',
      jsonInit('POST', req),
      'create export destination',
    ),
  )
}

export function useUpdateDataExportDestination() {
  return useInvalidatingMutation(
    (vars: { id: string; body: UpdateDataExportDestinationRequest }) =>
      request<DataExportDestination>(
        `/api/v1/export/destinations/${encodeURIComponent(vars.id)}`,
        jsonInit('PATCH', vars.body),
        'update export destination',
      ),
  )
}

export function useDeleteDataExportDestination() {
  return useInvalidatingMutation((id: string) =>
    request<void>(
      `/api/v1/export/destinations/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
      'delete export destination',
    ),
  )
}

export function useTestDataExportDestination() {
  return useInvalidatingMutation((id: string) =>
    request<DataExportTestResult>(
      `/api/v1/export/destinations/${encodeURIComponent(id)}/test`,
      { method: 'POST' },
      'test export destination',
    ),
  )
}
