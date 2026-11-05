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
  policy: ['silences', 'policy'] as const,
}

// The server's cap for security issues, from GET /api/v1/silences.
export interface SilencePolicy {
  protected_max_days?: number
  protected_keys?: string[]
}

const DURATION_HOURS: Record<SilenceDuration, number> = {
  '1h': 1,
  '1d': 24,
  '1w': 168,
  '30d': 720,
  forever: Number.POSITIVE_INFINITY,
}

// Durations the server will accept for issueKey, so the menu never offers one that fails.
export function allowedSilenceDurations(
  issueKey: string,
  policy: SilencePolicy | undefined,
): SilenceDuration[] {
  const days = policy?.protected_max_days ?? 0
  const keys = policy?.protected_keys ?? []
  const kind = issueKey.split(':')[0] ?? ''
  const protectedKey = keys.includes(kind) || keys.includes(issueKey)
  if (!protectedKey || days <= 0) return SILENCE_DURATIONS
  return SILENCE_DURATIONS.filter((d) => DURATION_HOURS[d] <= days * 24)
}

export function useSilencePolicy(enabled: boolean) {
  return useQuery({
    queryKey: silenceKeys.policy,
    enabled,
    retry: false,
    queryFn: async (): Promise<SilencePolicy> => {
      const res = await fetch('/api/v1/silences')
      if (!res.ok) throw await failure(res, 'fetch silence policy')
      return (await res.json()) as SilencePolicy
    },
  })
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
    meta: { errorToast: false },
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
