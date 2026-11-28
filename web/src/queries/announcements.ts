// Matches internal/api/announcements.go: the verified, signed announcement feed.

import { queryOptions } from '@tanstack/react-query'
import { useApiMutation } from './useApiMutation'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type AnnouncementTier =
  'critical' | 'advisory' | 'feature' | 'maintenance' | 'info' | 'newsletter'

export interface Announcement {
  id: string
  tier: AnnouncementTier
  title: string
  body: string
  applies_to?: string
  cta?: string
  links: { label?: string; url: string }[] | null
  issued_at: string
  expires_at: string
  read: boolean
  dismissed: boolean
  snoozed_until?: string
  dismissible: boolean
}

export interface AnnouncementStatus {
  mode: string
  sequence: number
  fetched_at: string
  verified_at: string
  last_error: string
  stale: boolean
  trust_configured: boolean
  source_url: string
}

export interface AnnouncementsResponse {
  items: Announcement[] | null
  status: AnnouncementStatus
}

export const announcementKeys = {
  all: ['announcements'] as const,
  list: () => [...announcementKeys.all, 'list'] as const,
  banner: () => [...announcementKeys.all, 'banner'] as const,
}

const BANNER_REFRESH_MS = 5 * 60_000

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function announcementsQueryOptions() {
  return queryOptions({
    queryKey: announcementKeys.list(),
    queryFn: async () => {
      const res = await fetch('/api/v1/announcements')
      if (!res.ok) return fail(res, 'fetch announcements')
      return (await res.json()) as AnnouncementsResponse
    },
    staleTime: 30_000,
  })
}

export function announcementBannerQueryOptions() {
  return queryOptions({
    queryKey: announcementKeys.banner(),
    queryFn: async () => {
      const res = await fetch('/api/v1/announcements/banner')
      if (!res.ok) return fail(res, 'fetch announcement banner')
      const body = (await res.json()) as { items?: Announcement[] | null }
      return body.items ?? []
    },
    retry: false,
    staleTime: BANNER_REFRESH_MS,
    refetchInterval: BANNER_REFRESH_MS,
  })
}

type Action = 'read' | 'dismiss' | 'snooze'

// A 409 on dismiss is the rule for critical notices, not a failure to toast.
export function useAnnouncementAction(action: Action) {
  return useApiMutation<void, string>({
    meta: { errorToast: action !== 'dismiss' },
    fn: async (id) => {
      const res = await fetch(
        `/api/v1/announcements/${encodeURIComponent(id)}/${action}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: action === 'snooze' ? '{}' : undefined,
        },
      )
      if (!res.ok) return fail(res, `${action} announcement`)
    },
    invalidate: [announcementKeys.all, ['attention-feed']],
  })
}
