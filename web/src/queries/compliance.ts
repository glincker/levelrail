import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Access reviews and evidence packs (internal/api/compliance.go).

export type AccessDecision = 'keep' | 'revoke'

export interface AccessReview {
  id: string
  name: string
  status: 'open' | 'completed'
  due_at: string
  overdue: boolean
  repeat_days: number
  created_by: string
  created_at: string
  completed_at?: string
  total: number
  decided: number
  to_revoke: number
}

export interface AccessReviewItem {
  id: string
  kind: 'user' | 'team_member' | 'api_token' | 'env_grant'
  subject_id: string
  label: string
  role: string
  detail: string
  last_active_at: string
  decision: AccessDecision | ''
  decided_by?: string
  applied_at?: string
  apply_error?: string
}

export interface AccessReviewDetail extends AccessReview {
  items: AccessReviewItem[]
}

export interface CompleteResult {
  revoked: number
  failures: AccessReviewItem[]
}

export interface EvidencePack {
  id: string
  range_from: string
  range_to: string
  status: 'running' | 'complete' | 'failed'
  step: string
  progress: number
  size_bytes: number
  sha256?: string
  error?: string
  created_by: string
  created_at: string
  finished_at?: string
}

export interface VerifyReport {
  valid: boolean
  signature_checked: boolean
  signature_valid: boolean
  key_id?: string
  files: { name: string; ok: boolean; reason?: string }[]
}

export const complianceKeys = {
  reviews: ['compliance', 'access-reviews'] as const,
  review: (id: string) => ['compliance', 'access-reviews', id] as const,
  packs: ['compliance', 'evidence-packs'] as const,
}

const BASE = '/api/v1/compliance'
const PACK_POLL_MS = 2000

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok && res.status !== 409) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${path} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  const data = (await res.json()) as T & { error?: string }
  if (res.status === 409 && !('failures' in (data as object))) {
    throw new ApiError(res.status, data.error ?? 'conflict')
  }
  return data
}

export function accessReviewsQueryOptions() {
  return queryOptions({
    queryKey: complianceKeys.reviews,
    queryFn: () => send<AccessReview[]>('GET', `${BASE}/access-reviews`),
  })
}

export function accessReviewQueryOptions(id: string) {
  return queryOptions({
    queryKey: complianceKeys.review(id),
    queryFn: () =>
      send<AccessReviewDetail>(
        'GET',
        `${BASE}/access-reviews/${encodeURIComponent(id)}`,
      ),
  })
}

export function useEvidencePacks() {
  return useQuery({
    queryKey: complianceKeys.packs,
    queryFn: () => send<EvidencePack[]>('GET', `${BASE}/evidence-packs`),
    refetchInterval: (q) =>
      q.state.data?.some((p) => p.status === 'running') ? PACK_POLL_MS : false,
  })
}

export function useStartAccessReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: {
      name: string
      due_days: number
      repeat_days: number
    }) => send<AccessReview>('POST', `${BASE}/access-reviews`, req),
    onSuccess: () => qc.invalidateQueries({ queryKey: complianceKeys.reviews }),
  })
}

export function useDecideItem(reviewId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { item: string; decision: AccessDecision }) =>
      send<void>(
        'PUT',
        `${BASE}/access-reviews/${encodeURIComponent(reviewId)}/items/${encodeURIComponent(v.item)}`,
        { decision: v.decision },
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: complianceKeys.review(reviewId) }),
  })
}

export function useCompleteReview(reviewId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      send<CompleteResult>(
        'POST',
        `${BASE}/access-reviews/${encodeURIComponent(reviewId)}/complete`,
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: complianceKeys.review(reviewId) })
      void qc.invalidateQueries({ queryKey: complianceKeys.reviews })
    },
  })
}

export function useCreatePack() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (range: { from: string; to: string }) =>
      send<EvidencePack>('POST', `${BASE}/evidence-packs`, range),
    onSuccess: () => qc.invalidateQueries({ queryKey: complianceKeys.packs }),
  })
}

export function useVerifyPack() {
  return useMutation({
    mutationFn: (id: string) =>
      send<VerifyReport>(
        'POST',
        `${BASE}/evidence-packs/${encodeURIComponent(id)}/verify`,
      ),
  })
}

export function packDownloadUrl(id: string): string {
  return `${BASE}/evidence-packs/${encodeURIComponent(id)}/download`
}
