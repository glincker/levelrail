// Query-key factory and fetchers for the per-app ingress abuse defense
// (internal/api/app_abuse.go): the policy, a status condition and a dry run
// that replays recent traffic against a draft.

import {
  keepPreviousData,
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AbuseDryRun, AbusePolicy, AppAbuse } from '../types/abuse'
import { appKeys } from './apps'

const REFETCH_MS = 15_000

export const abuseKeys = {
  policy: (app: string) => [...appKeys.detail(app), 'abuse'] as const,
  dryRun: (app: string, draft: string) =>
    [...appKeys.detail(app), 'abuse-dry-run', draft] as const,
}

function path(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/abuse`
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchAbuse(app: string): Promise<AppAbuse> {
  const res = await fetch(path(app))
  if (!res.ok) throw await failure(res, 'fetch abuse defense')
  return (await res.json()) as AppAbuse
}

export async function saveAbuse(
  app: string,
  policy: AbusePolicy,
): Promise<AppAbuse> {
  const res = await fetch(path(app), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(policy),
  })
  if (!res.ok) throw await failure(res, 'save abuse defense')
  return (await res.json()) as AppAbuse
}

export async function fetchAbuseDryRun(
  app: string,
  policy: AbusePolicy,
): Promise<AbuseDryRun> {
  const res = await fetch(`${path(app)}/dry-run`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ policy }),
  })
  if (!res.ok) throw await failure(res, 'abuse defense preview')
  return (await res.json()) as AbuseDryRun
}

export function useAppAbuse(app: string) {
  return useQuery({
    queryKey: abuseKeys.policy(app),
    queryFn: () => fetchAbuse(app),
    refetchInterval: REFETCH_MS,
  })
}

/** useAbuseDryRun replays recent traffic against the draft; null skips the call. */
export function useAbuseDryRun(app: string, draft: AbusePolicy | null) {
  const key = draft ? JSON.stringify(draft) : ''
  return useQuery({
    queryKey: abuseKeys.dryRun(app, key),
    queryFn: draft ? () => fetchAbuseDryRun(app, draft) : skipToken,
    placeholderData: keepPreviousData,
    staleTime: 0,
    retry: false,
  })
}

export function useSaveAbuse(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (policy: AbusePolicy) => saveAbuse(app, policy),
    onSuccess: (data) => {
      qc.setQueryData(abuseKeys.policy(app), data)
    },
  })
}
