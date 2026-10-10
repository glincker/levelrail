import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/attention's Item as GET /api/v1/attention/feed returns
// it: items from sources that need a store query, already limited to what
// this session may see.
export interface AttentionFeedItem {
  id: string
  severity: 'critical' | 'warning' | 'info'
  kind: string
  subject: string
  title: string
  detail: string
  action: string
  link: string
  params?: Record<string, string>
}

export const attentionFeedKeys = {
  all: ['attention-feed'] as const,
}

export async function fetchAttentionFeed(): Promise<AttentionFeedItem[]> {
  const res = await fetch('/api/v1/attention/feed')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch attention feed failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { items?: AttentionFeedItem[] }
  return Array.isArray(body.items) ? body.items : []
}

export function attentionFeedQueryOptions() {
  return queryOptions({
    queryKey: attentionFeedKeys.all,
    queryFn: fetchAttentionFeed,
  })
}
