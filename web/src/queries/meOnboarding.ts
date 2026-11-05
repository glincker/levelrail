// Per-user first-run flow: GET/PUT /api/v1/me/onboarding and POST .../reset.
// Not the platform setup wizard at /api/v1/onboarding.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'

export type OnboardingState =
  'not_started' | 'in_progress' | 'snoozed' | 'completed' | 'dismissed'

export interface MeOnboarding {
  state: OnboardingState
  current_step: string
  flow_version: number
  enabled: boolean
  started_at?: string | null
  completed_at?: string | null
  dismissed_at?: string | null
  milestone?: string
  next_milestone?: string
  restore_due?: boolean
}

export interface OnboardingUpdate {
  state: OnboardingState
  current_step?: string
}

export const meOnboardingKey = ['me', 'onboarding'] as const

const ENDPOINT = '/api/v1/me/onboarding'

/** OnboardingTransitionError is the server refusing an illegal state move (409 invalid_transition). */
export class OnboardingTransitionError extends ApiError {
  constructor(message: string) {
    super(409, message)
    this.name = 'OnboardingTransitionError'
  }
}

async function failure(res: Response, what: string): Promise<ApiError> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    code?: string
  } | null
  const message = body?.error ?? `${what} failed: ${String(res.status)}`
  if (res.status === 409 && body?.code === 'invalid_transition') {
    return new OnboardingTransitionError(message)
  }
  return new ApiError(res.status, message)
}

/** fetchMeOnboarding resolves null when the route is missing (older server) so callers stay quiet. */
export async function fetchMeOnboarding(): Promise<MeOnboarding | null> {
  const res = await fetch(ENDPOINT, { cache: 'no-store' })
  if (res.status === 404) return null
  if (!res.ok) throw await failure(res, 'fetch onboarding')
  return (await res.json()) as MeOnboarding
}

export function meOnboardingQueryOptions() {
  return queryOptions({
    queryKey: meOnboardingKey,
    queryFn: fetchMeOnboarding,
    staleTime: 30_000,
  })
}

/** useMeOnboarding never throws and never retries: the flow is optional and must not break the page it sits on. */
export function useMeOnboarding() {
  return useQuery({ ...meOnboardingQueryOptions(), retry: false })
}

/** onboardingActive is true only when the server has the flow and it is switched on. */
export function onboardingActive(
  data: MeOnboarding | null | undefined,
): data is MeOnboarding {
  return data != null && data.enabled
}

/** useOnboardingCardShown tells the dashboard whether the setup card is the page's primary action. */
export function useOnboardingCardShown(): boolean {
  const { data } = useMeOnboarding()
  return (
    onboardingActive(data) &&
    (data.state === 'not_started' || data.state === 'in_progress')
  )
}

export function useSetOnboarding() {
  const qc = useQueryClient()
  return useMutation<MeOnboarding, ApiError, OnboardingUpdate>({
    mutationFn: async (update) => {
      const res = await fetch(ENDPOINT, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(update),
      })
      if (!res.ok) throw await failure(res, 'update onboarding')
      return (await res.json()) as MeOnboarding
    },
    onSuccess: (next) => {
      qc.setQueryData(meOnboardingKey, next)
    },
  })
}

export function useResetOnboarding() {
  const qc = useQueryClient()
  return useMutation<MeOnboarding, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(`${ENDPOINT}/reset`, { method: 'POST' })
      if (!res.ok) throw await failure(res, 'reset onboarding')
      return (await res.json()) as MeOnboarding
    },
    onSuccess: (next) => {
      qc.setQueryData(meOnboardingKey, next)
    },
  })
}
