// Delivery tab data: the merged app timeline (cursor paged) and the app's connection bindings.

import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '../lib/pollTier'
import type {
  AppBinding,
  AppBindingsResponse,
  TimelinePage,
} from '../types/appDelivery'

export const appDeliveryKeys = {
  all: ['appDelivery'] as const,
  list: (app: string) => [...appDeliveryKeys.all, 'list', app] as const,
  bindings: (app: string) => [...appDeliveryKeys.all, 'bindings', app] as const,
}

export const TIMELINE_PAGE_SIZE = 50

// The Delivery tab refreshes on the shared "active" cadence; react-query pauses it in hidden tabs.
export function deliveryRefreshMs(): number {
  return pollMs('active')
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

// A null body or null items is an empty page, never a crash.
export async function fetchTimelinePage(
  app: string,
  before?: string,
): Promise<TimelinePage> {
  const q = new URLSearchParams({ limit: String(TIMELINE_PAGE_SIZE) })
  if (before) q.set('before', before)
  const body = await getJson<Partial<TimelinePage> | null>(
    `/api/v1/apps/${encodeURIComponent(app)}/timeline?${q.toString()}`,
    'fetch app timeline',
  )
  return {
    items: body?.items ?? [],
    next_cursor: body?.next_cursor || undefined,
  }
}

export async function fetchAppBindings(app: string): Promise<AppBinding[]> {
  const body = await getJson<AppBindingsResponse | null>(
    `/api/v1/apps/${encodeURIComponent(app)}/bindings`,
    'fetch app bindings',
  )
  return body?.bindings ?? []
}

export function useAppDelivery(app: string) {
  return useInfiniteQuery({
    queryKey: appDeliveryKeys.list(app),
    queryFn: ({ pageParam }) => fetchTimelinePage(app, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    refetchInterval: deliveryRefreshMs(),
    enabled: app !== '',
  })
}

export function useAppBindings(app: string) {
  return useQuery({
    queryKey: appDeliveryKeys.bindings(app),
    queryFn: () => fetchAppBindings(app),
    refetchInterval: deliveryRefreshMs(),
    enabled: app !== '',
  })
}
