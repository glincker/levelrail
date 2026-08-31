// Query-key factory, fetcher, and complete-mutation for the first-run
// onboarding flag: GET /api/v1/onboarding and POST
// /api/v1/onboarding/complete (internal/api/onboarding.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface OnboardingState {
  completed: boolean
}

export const onboardingKeys = {
  all: ['onboarding'] as const,
}

export async function fetchOnboardingState(): Promise<OnboardingState> {
  const res = await fetch('/api/v1/onboarding')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch onboarding state failed: ${res.status}`),
    )
  }
  return (await res.json()) as OnboardingState
}

export function onboardingQueryOptions() {
  return queryOptions({
    queryKey: onboardingKeys.all,
    queryFn: fetchOnboardingState,
    staleTime: 60_000,
  })
}

// Not suspense: the dashboard's own empty state must render even if this
// fetch is slow or fails, same "never block the page's core function"
// reasoning useSystemStatusOptional already establishes for its own
// optional signal. A failed/pending fetch just means the onboarding
// checklist doesn't show yet, not that the dashboard itself breaks.
export function useOnboardingStateOptional() {
  return useQuery({ ...onboardingQueryOptions(), retry: false })
}

export async function completeOnboarding(): Promise<OnboardingState> {
  const res = await fetch('/api/v1/onboarding/complete', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `complete onboarding failed: ${res.status}`),
    )
  }
  return (await res.json()) as OnboardingState
}

export function useCompleteOnboarding() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: completeOnboarding,
    onSuccess: (data) => {
      queryClient.setQueryData(onboardingKeys.all, data)
    },
  })
}
