import {
  useMutation,
  useQuery,
  useQueryClient,
  keepPreviousData,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type SearchKind =
  | 'app'
  | 'database'
  | 'node'
  | 'domain'
  | 'template'
  | 'project'
  | 'environment'
  | 'tag'
  | 'deployment'
  | 'setting'
  | 'doc'
  | 'action'
  | 'saved'

// Matches internal/search's ActionInfo.
export interface SearchActionInfo {
  id: string
  ability: string
  confirm: boolean
}

// Matches internal/search's Hit.
export interface SearchHit {
  ref: string
  kind: SearchKind
  title: string
  subtitle?: string
  app_name?: string
  env?: string
  node_name?: string
  tags?: string[]
  href?: string
  score: number
  action?: SearchActionInfo
}

export interface SearchResponse {
  hits: SearchHit[]
  next_cursor?: string
  total: number
}

export interface SearchSuggestion {
  text: string
  kind: string
  ref?: string
}

export interface SavedSearch {
  id: string
  name: string
  query: string
  created_at: string
}

const BASE = '/api/v1/search'

async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { signal, cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `search failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export interface SearchParams {
  q: string
  kind?: SearchKind
  limit?: number
  cursor?: string
}

export function searchUrl(p: SearchParams): string {
  const v = new URLSearchParams({ q: p.q })
  if (p.kind) v.set('kind', p.kind)
  if (p.limit) v.set('limit', String(p.limit))
  if (p.cursor) v.set('cursor', p.cursor)
  return `${BASE}?${v.toString()}`
}

export function fetchSearch(
  p: SearchParams,
  signal?: AbortSignal,
): Promise<SearchResponse> {
  return getJSON<SearchResponse>(searchUrl(p), signal)
}

export function useSearch(p: SearchParams, enabled = true) {
  return useQuery({
    queryKey: ['search', p.q, p.kind ?? '', p.limit ?? 0, p.cursor ?? ''],
    queryFn: ({ signal }) => fetchSearch(p, signal),
    enabled: enabled && p.q.trim() !== '',
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  })
}

export function useSearchSuggest(q: string, enabled = true) {
  return useQuery({
    queryKey: ['search-suggest', q],
    queryFn: ({ signal }) =>
      getJSON<{ suggestions: SearchSuggestion[] }>(
        `${BASE}/suggest?q=${encodeURIComponent(q)}`,
        signal,
      ).then((r) => r.suggestions),
    enabled,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  })
}

export function useSavedSearches() {
  return useQuery({
    queryKey: ['search-saved'],
    queryFn: ({ signal }) =>
      getJSON<{ saved: SavedSearch[] }>(`${BASE}/saved`, signal).then(
        (r) => r.saved,
      ),
    staleTime: 30_000,
  })
}

async function send(
  url: string,
  method: 'POST' | 'DELETE',
  body?: unknown,
): Promise<Response> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `search request failed: ${res.status}`),
    )
  }
  return res
}

export function useSaveSearch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { name: string; query: string }) =>
      (await (await send(`${BASE}/saved`, 'POST', v)).json()) as SavedSearch,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['search-saved'] }),
  })
}

export function useDeleteSavedSearch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await send(`${BASE}/saved/${encodeURIComponent(id)}`, 'DELETE')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['search-saved'] }),
  })
}

// Best effort: a read-only caller gets 403 and the boost simply stays off.
export async function recordSearchUse(ref: string): Promise<void> {
  try {
    await fetch(`${BASE}/usage`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ref }),
    })
  } catch {
    // ignored on purpose
  }
}
