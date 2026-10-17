import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/plugin's Record plus the API's running flag.
export interface Plugin {
  name: string
  version: string
  kind: string
  publisher: string
  description: string
  image: string
  digest: string
  manifest: string
  key_id: string
  requested_abilities: string[]
  approved_abilities: string[]
  outbound_hosts: string[]
  config_json: string
  state: 'installed' | 'enabled' | 'disabled' | 'removing'
  running: boolean
  approved_at: string
  approved_by: string
  installed_at: string
  installed_by: string
  last_error: string
}

// Matches internal/plugin's TrustedKey.
export interface PluginKey {
  key_id: string
  name: string
  public_key: string
  added_at: string
  added_by: string
}

export interface InstallPluginInput {
  manifest: string
  signature: string
  key_id: string
  expected_digest?: string
  config_json?: string
}

export const pluginKeys = {
  all: ['plugins'] as const,
  keys: ['plugin-keys'] as const,
}

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    cache: 'no-store',
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function pluginsQueryOptions() {
  return queryOptions({
    queryKey: pluginKeys.all,
    queryFn: () => send<Plugin[]>('GET', '/api/v1/plugins'),
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
}

export function pluginKeysQueryOptions() {
  return queryOptions({
    queryKey: pluginKeys.keys,
    queryFn: () => send<PluginKey[]>('GET', '/api/v1/plugin-keys'),
    staleTime: 15_000,
  })
}

export function usePlugins() {
  return useQuery(pluginsQueryOptions())
}

export function usePluginKeys() {
  return useQuery(pluginKeysQueryOptions())
}

function useInvalidating<TVars, TData>(
  fn: (vars: TVars) => Promise<TData>,
  keys: readonly (readonly string[])[],
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      for (const queryKey of keys) void qc.invalidateQueries({ queryKey })
    },
  })
}

const plugin = (name: string) => `/api/v1/plugins/${encodeURIComponent(name)}`

export function useInstallPlugin() {
  return useInvalidating(
    (input: InstallPluginInput) =>
      send<Plugin>('POST', '/api/v1/plugins', input),
    [pluginKeys.all],
  )
}

export function useApprovePlugin() {
  return useInvalidating(
    (v: { name: string; abilities: string[] }) =>
      send<Plugin>('POST', `${plugin(v.name)}/approve`, {
        abilities: v.abilities,
      }),
    [pluginKeys.all],
  )
}

export function usePluginAction(action: 'enable' | 'disable') {
  return useInvalidating(
    (name: string) => send<Plugin>('POST', `${plugin(name)}/${action}`),
    [pluginKeys.all],
  )
}

export function useRemovePlugin() {
  return useInvalidating(
    (name: string) => send<void>('DELETE', plugin(name)),
    [pluginKeys.all],
  )
}

export function useAddPluginKey() {
  return useInvalidating(
    (v: { name: string; public_key: string }) =>
      send<PluginKey>('POST', '/api/v1/plugin-keys', v),
    [pluginKeys.keys],
  )
}

export function useRemovePluginKey() {
  return useInvalidating(
    (keyId: string) =>
      send<void>('DELETE', `/api/v1/plugin-keys/${encodeURIComponent(keyId)}`),
    [pluginKeys.keys, pluginKeys.all],
  )
}
