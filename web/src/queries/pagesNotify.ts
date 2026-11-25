import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys } from './staticSites'
import { useApiMutation } from './useApiMutation'

export type NotifyEvent = 'success' | 'failure'

export interface PagesNotifyChannel {
  target_id: string
  channel_id: string
  enabled: boolean
}

export interface PagesNotify {
  site: string
  enabled: boolean
  events: NotifyEvent[]
  explicit: boolean
  channels: PagesNotifyChannel[]
}

export interface PagesNotifyUpdate {
  enabled?: boolean
  events?: NotifyEvent[]
  channel_id?: string
}

export interface NotifyDraft {
  enabled: boolean
  success: boolean
  failure: boolean
  channelId: string
}

const root = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}/notify`

const notifyKey = (name: string) =>
  [...staticSitesKeys.all, 'notify', name] as const

/** Seeds the form from the stored view; an unconfigured site starts on with both events. */
export function draftFromNotify(n: PagesNotify | undefined): NotifyDraft {
  const events: NotifyEvent[] = n?.events ?? ['success', 'failure']
  return {
    enabled: n?.enabled ?? true,
    success: events.includes('success'),
    failure: events.includes('failure'),
    channelId: '',
  }
}

/** The server rejects an empty event list, so the form blocks saving it. */
export function draftIsValid(d: NotifyDraft): boolean {
  return d.success || d.failure
}

export function draftToUpdate(d: NotifyDraft): PagesNotifyUpdate {
  const events: NotifyEvent[] = []
  if (d.success) events.push('success')
  if (d.failure) events.push('failure')
  return {
    enabled: d.enabled,
    events,
    ...(d.channelId ? { channel_id: d.channelId } : {}),
  }
}

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function pagesNotifyQueryOptions(name: string) {
  return queryOptions({
    queryKey: notifyKey(name),
    queryFn: async (): Promise<PagesNotify> => {
      const res = await fetch(root(name))
      if (!res.ok) return fail(res, 'notify')
      return (await res.json()) as PagesNotify
    },
    staleTime: 30_000,
    retry: false,
  })
}

export function usePagesNotify(name: string) {
  return useQuery(pagesNotifyQueryOptions(name))
}

export function useSavePagesNotify(name: string) {
  return useApiMutation<PagesNotify, PagesNotifyUpdate>({
    fn: async (body) => {
      const res = await fetch(root(name), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) return fail(res, 'notify')
      return (await res.json()) as PagesNotify
    },
    invalidate: [notifyKey(name)],
  })
}
