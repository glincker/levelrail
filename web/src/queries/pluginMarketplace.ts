import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pluginKeys, type Plugin } from './plugins'

export type MarketplaceErrorKind =
  '' | 'fetch' | 'signature' | 'format' | 'replay' | 'too_large'

// Matches internal/plugin's CatalogEntry.
export interface MarketplaceEntry {
  id: string
  name: string
  publisher: string
  publisher_key_id: string
  publisher_key_name: string
  version: string
  summary: string
  categories: string[]
  icon: string
  abilities: string[]
  outbound_hosts: string[]
  digest: string
  changelog: string
  min_version: string
  source_id: string
  source_name: string
  verified: boolean
  key_trusted: boolean
  installed: boolean
  installed_version: string
  update_available: boolean
  compatible: boolean
}

// Matches internal/plugin's SourceStatus.
export interface MarketplaceSource {
  id: string
  name: string
  url: string
  root_key_id: string
  builtin: boolean
  status: 'ok' | 'stale' | 'error'
  error: string
  error_kind: MarketplaceErrorKind
  sequence: number
  generated_at: string
  fetched_at: string
  plugin_count: number
}

export interface MarketplaceCatalog {
  entries: MarketplaceEntry[]
  sources: MarketplaceSource[]
}

// Matches internal/plugin's MarketPreview.
export interface MarketplacePreview {
  entry: Pick<
    MarketplaceEntry,
    'id' | 'name' | 'publisher' | 'version' | 'summary' | 'icon' | 'changelog'
  > & { abilities: string[]; outbound_hosts: string[]; digest: string }
  source_id: string
  source_name: string
  key_id: string
  key_name: string
  key_trusted: boolean
  action: 'install' | 'upgrade'
  installed_version: string
  added_abilities: string[]
  added_hosts: string[]
  reapproval_required: boolean
}

export interface MarketplaceInstallInput {
  source_id: string
  id: string
  version: string
  digest: string
  abilities: string[]
  trust_key_id?: string
}

export interface MarketplaceInstallResult {
  plugin: Plugin
  action: 'install' | 'upgrade'
  trust_added: boolean
}

export const marketplaceKeys = {
  catalog: ['plugin-marketplace'] as const,
}

const base = '/api/v1/plugin-marketplace'

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

export function marketplaceQueryOptions() {
  return queryOptions({
    queryKey: marketplaceKeys.catalog,
    queryFn: () => send<MarketplaceCatalog>('GET', base),
    staleTime: 60_000,
  })
}

export function useMarketplace() {
  return useQuery(marketplaceQueryOptions())
}

function useCatalogMutation<TVars, TData>(
  fn: (vars: TVars) => Promise<TData>,
  alsoInvalidate: readonly (readonly string[])[] = [],
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: marketplaceKeys.catalog })
      for (const queryKey of alsoInvalidate)
        void qc.invalidateQueries({ queryKey })
    },
  })
}

export function useRefreshMarketplace() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (sourceId: string) =>
      send<MarketplaceCatalog>('POST', `${base}/refresh`, {
        source_id: sourceId,
      }),
    onSuccess: (data) => qc.setQueryData(marketplaceKeys.catalog, data),
  })
}

export function useMarketplacePreview() {
  return useMutation({
    mutationFn: (v: { source_id: string; id: string }) =>
      send<MarketplacePreview>('POST', `${base}/preview`, v),
  })
}

export function useMarketplaceInstall() {
  return useCatalogMutation(
    (input: MarketplaceInstallInput) =>
      send<MarketplaceInstallResult>('POST', `${base}/install`, input),
    [pluginKeys.all, pluginKeys.keys],
  )
}

export function useAddMarketplaceSource() {
  return useCatalogMutation(
    (v: { name: string; url: string; root_public_key: string }) =>
      send<MarketplaceSource>('POST', `${base}/sources`, v),
  )
}

export function useRemoveMarketplaceSource() {
  return useCatalogMutation((id: string) =>
    send<void>('DELETE', `${base}/sources/${encodeURIComponent(id)}`),
  )
}
