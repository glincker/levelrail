import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { attentionFeedKeys } from './attentionFeed'

export type SilenceDuration = '1h' | '1d' | '1w' | '30d' | 'forever'

export const SILENCE_DURATIONS: SilenceDuration[] = [
  '1h',
  '1d',
  '1w',
  '30d',
  'forever',
]

// Mirrors internal/store's Silence as GET /api/v1/silences returns it.
export interface IssueSilence {
  id: string
  issue_key: string
  scope?: string
  until?: string | null
  severity?: string
  created_by: string
  reason?: string
  created_at: string
  expired: boolean
}

export interface CreateSilenceInput {
  issueKey: string
  duration: SilenceDuration
  severity?: string
  scope?: string
  reason?: string
  confirm?: boolean
}

export const silenceKeys = {
  all: ['silences'] as const,
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchSilences(): Promise<IssueSilence[]> {
  const res = await fetch('/api/v1/silences')
  if (!res.ok) throw await failure(res, 'fetch silences')
  const body = (await res.json()) as { silences?: IssueSilence[] }
  return Array.isArray(body.silences) ? body.silences : []
}

export function silencesQueryOptions() {
  return queryOptions({ queryKey: silenceKeys.all, queryFn: fetchSilences })
}

export function isSilenceActive(s: IssueSilence, now: number): boolean {
  if (s.expired) return false
  return !s.until || now < Date.parse(s.until)
}

export function useSilences() {
  return useQuery({
    ...silencesQueryOptions(),
    retry: false,
    refetchInterval: 30_000,
  })
}

export function useCreateSilence() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: CreateSilenceInput): Promise<IssueSilence> => {
      const res = await fetch('/api/v1/silences', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          issue_key: input.issueKey,
          duration: input.duration,
          severity: input.severity,
          scope: input.scope,
          reason: input.reason,
          confirm: input.confirm,
        }),
      })
      if (!res.ok) throw await failure(res, 'silence issue')
      return (await res.json()) as IssueSilence
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: silenceKeys.all })
      void qc.invalidateQueries({ queryKey: attentionFeedKeys.all })
    },
  })
}

export function useDeleteSilence() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const res = await fetch(`/api/v1/silences/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw await failure(res, 'unsilence issue')
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: silenceKeys.all })
      void qc.invalidateQueries({ queryKey: attentionFeedKeys.all })
    },
  })
}
