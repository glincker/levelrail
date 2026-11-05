import { useEffect, useSyncExternalStore } from 'react'
import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import {
  createAppearanceSync,
  type SyncApi,
  type SyncState,
} from '../lib/appearanceSync'

// Account appearance: internal/api/me_preferences.go.
const ENDPOINT = '/api/v1/me/preferences/appearance'

export type AccountAppearance =
  { available: false } | { available: true; value: unknown }

async function failure(res: Response): Promise<ApiError> {
  const body = (await res.json().catch(() => null)) as { error?: string } | null
  return new ApiError(
    res.status,
    body?.error ?? `appearance sync failed: ${String(res.status)}`,
  )
}

/** fetchAccountAppearance reports unavailable (not an error) for 403/404 so the UI hides quietly. */
export async function fetchAccountAppearance(): Promise<AccountAppearance> {
  const res = await fetch(ENDPOINT, { cache: 'no-store' })
  if (res.status === 404 || res.status === 403) return { available: false }
  if (!res.ok) throw await failure(res)
  const body = (await res.json()) as { value?: unknown } | null
  return { available: true, value: body?.value ?? null }
}

export const appearanceSyncApi: SyncApi = {
  put: async (body) => {
    const res = await fetch(ENDPOINT, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw await failure(res)
  },
  del: async () => {
    const res = await fetch(ENDPOINT, { method: 'DELETE' })
    if (!res.ok && res.status !== 404) throw await failure(res)
  },
}

export const appearanceSync = createAppearanceSync(appearanceSyncApi)

export const accountAppearanceKey = ['me', 'preferences', 'appearance'] as const

export function accountAppearanceQueryOptions() {
  return queryOptions({
    queryKey: accountAppearanceKey,
    queryFn: fetchAccountAppearance,
    staleTime: Infinity,
    retry: false,
  })
}

/** useAppearanceSyncRunner mounts once in the signed-in shell; it never blocks rendering. */
export function useAppearanceSyncRunner() {
  const { data } = useQuery(accountAppearanceQueryOptions())
  useEffect(() => appearanceSync.start(), [])
  useEffect(() => {
    if (!data) return
    appearanceSync.setAvailable(data.available)
    if (data.available) appearanceSync.reconcile(data.value)
  }, [data])
}

export function useAppearanceSyncState(): SyncState {
  return useSyncExternalStore(
    appearanceSync.subscribe,
    appearanceSync.getState,
    appearanceSync.getState,
  )
}
