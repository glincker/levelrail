import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type PluginHealth = 'healthy' | 'unhealthy' | 'stopped' | 'removing'
export type PluginFix = '' | 'restart' | 'approve' | 'trust_key' | 'reinstall'
export type PluginVerificationStatus =
  | 'verified'
  | 'unsigned'
  | 'unsigned_refused'
  | 'untrusted_key'
  | 'bad_signature'
  | 'digest_mismatch'
  | 'unverified'

export interface PluginPanel {
  id: string
  title: string
  description: string
  rows: { label: string; value: string }[]
}

export interface PluginVerification {
  status: PluginVerificationStatus
  digest_pinned: boolean
  digest: string
  signed: boolean
  key_id: string
  key_name: string
  installable: boolean
  reason: string
}

export interface PluginPreview {
  manifest: {
    name: string
    version: string
    kind: string
    publisher: string
    description: string
    image: string
    abilities: string[]
    outbound_hosts: string[]
    contributes: { panels: PluginPanel[] }
  }
  verification: PluginVerification
  action: 'install' | 'upgrade'
  installed_version: string
  approved_abilities: string[]
  added_abilities: string[]
  removed_abilities: string[]
  added_hosts: string[]
  removed_hosts: string[]
  reapproval_required: boolean
}

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
  health: PluginHealth
  verification: PluginVerificationStatus
  fix: PluginFix
  panels: PluginPanel[]
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
  manifest?: string
  manifest_url?: string
  signature?: string
  key_id?: string
  expected_digest?: string
  config_json?: string
  approve_abilities?: string[]
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

export function useInspectPlugin() {
  return useMutation({
    mutationFn: (input: InstallPluginInput) =>
      send<PluginPreview>('POST', '/api/v1/plugins/inspect', input),
  })
}

export function useUpgradePlugin() {
  return useInvalidating(
    (v: { name: string; input: InstallPluginInput }) =>
      send<Plugin>('POST', `${plugin(v.name)}/upgrade`, v.input),
    [pluginKeys.all],
  )
}

export function useRestartPlugin() {
  return useInvalidating(
    (name: string) => send<Plugin>('POST', `${plugin(name)}/restart`),
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
