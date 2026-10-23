// GET /api/v1/functions and /functions/runtimes (internal/api/functions.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys, createApp } from './apps'
import { triggerBuild } from './builds'

export type FunctionWindow = '1h' | '24h' | '7d'

export interface FunctionItem {
  name: string
  image: string
  port: number
  idle_minutes: number
  sleeping: boolean
  url?: string
  invocations: number
  error_rate_5xx: number
  p95_ms: number
}

export interface FunctionsList {
  window_seconds: number
  functions: FunctionItem[]
}

export interface FunctionRuntime {
  id: string
  name: string
  files: { path: string; content: string }[]
}

export interface FunctionRuntimes {
  runtimes: FunctionRuntime[]
  port: number
}

export const functionKeys = {
  all: ['functions'] as const,
  list: (window: FunctionWindow) => ['functions', 'list', window] as const,
  runtimes: ['functions', 'runtimes'] as const,
}

async function getJSON<T>(path: string, what: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function functionsQueryOptions(window: FunctionWindow) {
  return queryOptions({
    queryKey: functionKeys.list(window),
    queryFn: () =>
      getJSON<FunctionsList>(
        `/api/v1/functions?window=${window}`,
        'fetch functions',
      ),
    refetchInterval: 30_000,
  })
}

export function useFunctions(window: FunctionWindow) {
  return useQuery(functionsQueryOptions(window))
}

export function useFunctionRuntimes() {
  return useQuery({
    queryKey: functionKeys.runtimes,
    queryFn: () =>
      getJSON<FunctionRuntimes>(
        '/api/v1/functions/runtimes',
        'fetch function runtimes',
      ),
    staleTime: Infinity,
  })
}

export interface DeployFunctionInput {
  name: string
  repoUrl: string
  ref: string
  directory: string
  /** A prebuilt registry reference; when set, nothing is cloned or built. */
  image?: string
  port: number
  idleMinutes: number
  routeDomain?: string
  routePath?: string
}

export interface FunctionRoute {
  domain: string
  path: string
  strip_prefix: boolean
}

export async function setFunctionRoute(
  name: string,
  route: FunctionRoute,
): Promise<void> {
  const res = await fetch(
    `/api/v1/functions/${encodeURIComponent(name)}/route`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(route),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set function route failed: ${res.status}`),
    )
  }
}

async function setSleepHold(name: string, idleMinutes: number): Promise<void> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/sleep`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ idle_minutes: idleMinutes, hold_requests: true }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `enable sleep failed: ${res.status}`),
    )
  }
}

async function appExists(name: string): Promise<boolean> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}`)
  return res.ok
}

/** Creates the app if needed, turns on sleep with request hold, then builds the handler with Railpack. */
export async function deployFunction(
  input: DeployFunctionInput,
): Promise<void> {
  if (!(await appExists(input.name))) {
    await createApp({
      name: input.name,
      image: input.image || `${input.name}:pending`,
      port: input.port,
      env: { PORT: String(input.port) },
    })
  }
  await setSleepHold(input.name, input.idleMinutes)
  if (input.routeDomain && input.routePath) {
    await setFunctionRoute(input.name, {
      domain: input.routeDomain,
      path: input.routePath,
      strip_prefix: true,
    })
  }
  if (input.image) {
    await triggerBuild(input.name, {
      repoUrl: '',
      ref: '',
      buildType: 'image',
      image: input.image,
    })
    return
  }
  await triggerBuild(input.name, {
    repoUrl: input.repoUrl,
    ref: input.ref,
    imageRepo: input.name,
    buildType: 'railpack',
    baseDirectory: input.directory || undefined,
  })
}

export function useDeployFunction() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, DeployFunctionInput>({
    mutationFn: deployFunction,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: functionKeys.all })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
