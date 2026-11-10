// GET/PUT /api/v1/apps/{name}/services/{service}/runtime
// (internal/api/app_service_runtime.go): allowlisted container options with
// the live bounds. PUT replaces the editable keys and needs the read ETag.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface RuntimeUlimit {
  soft: number
  hard: number
}

export interface RuntimeOptionValues {
  tmpfs?: string[]
  ulimits?: Record<string, RuntimeUlimit>
  init?: boolean
  stop_timeout?: number
  extra_hosts?: string[]
  dns?: string[]
  sysctls?: Record<string, string>
  pids_limit?: number
}

export interface RuntimeBounds {
  tmpfs_max: number
  tmpfs_bytes_max: number
  extra_hosts_max: number
  dns_max: number
  stop_timeout_max_seconds: number
  pids_limit_max: number
  ulimit_count_max: number
  ulimit_bytes_max: number
  ulimits: string[]
  sysctls: string[]
}

export interface RuntimeOptionsResource {
  app: string
  service: string
  revision: string
  runtime: RuntimeOptionValues
  editable: string[]
  bounds: RuntimeBounds
}

// RuntimeOptionsError carries the per-key messages of a 422.
export class RuntimeOptionsError extends ApiError {
  readonly fieldErrors: Record<string, string>

  constructor(
    status: number,
    message: string,
    fieldErrors: Record<string, string> = {},
  ) {
    super(status, message)
    this.fieldErrors = fieldErrors
  }
}

export const runtimeOptionsKeys = {
  detail: (app: string, service: string) =>
    ['runtime-options', app, service] as const,
}

function path(app: string, service: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/services/${encodeURIComponent(service)}/runtime`
}

export async function fetchRuntimeOptions(
  app: string,
  service: string,
): Promise<RuntimeOptionsResource> {
  const res = await fetch(path(app, service))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `load runtime options failed: ${res.status}`),
    )
  }
  return (await res.json()) as RuntimeOptionsResource
}

export function runtimeOptionsQueryOptions(app: string, service: string) {
  return queryOptions({
    queryKey: runtimeOptionsKeys.detail(app, service),
    queryFn: () => fetchRuntimeOptions(app, service),
    staleTime: Infinity,
  })
}

export async function putRuntimeOptions(
  app: string,
  service: string,
  revision: string,
  values: RuntimeOptionValues,
): Promise<RuntimeOptionsResource> {
  const res = await fetch(path(app, service), {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'If-Match': `"${revision}"`,
    },
    body: JSON.stringify(values),
  })
  if (!res.ok) {
    const body = (await res
      .clone()
      .json()
      .catch(() => null)) as { errors?: Record<string, string> } | null
    throw new RuntimeOptionsError(
      res.status,
      await readErrorMessage(res, `save runtime options failed: ${res.status}`),
      body?.errors ?? {},
    )
  }
  return (await res.json()) as RuntimeOptionsResource
}

export function useSetRuntimeOptions(app: string, service: string) {
  const queryClient = useQueryClient()
  return useMutation<
    RuntimeOptionsResource,
    RuntimeOptionsError,
    { revision: string; values: RuntimeOptionValues }
  >({
    mutationFn: ({ revision, values }) =>
      putRuntimeOptions(app, service, revision, values),
    onSuccess: (next) =>
      queryClient.setQueryData(runtimeOptionsKeys.detail(app, service), next),
    onError: (e) => {
      // A stale revision means the cached copy is wrong; read it again.
      if (e.status === 412) {
        void queryClient.invalidateQueries({
          queryKey: runtimeOptionsKeys.detail(app, service),
        })
      }
    },
  })
}
