// Query-key factory, fetcher, and mutations for the setup wizard's
// server-side state (internal/api/onboarding.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { SetupStepId, SetupStepStatus } from '../lib/setupWizard'

export interface OnboardingState {
  completed: boolean
  current_step: SetupStepId | ''
  steps: Partial<Record<SetupStepId, SetupStepStatus>>
}

export interface OnboardingProgress {
  current_step: SetupStepId
  steps: Partial<Record<SetupStepId, SetupStepStatus>>
}

export const onboardingKeys = {
  all: ['onboarding'] as const,
}

async function readState(
  res: Response,
  fallback: string,
): Promise<OnboardingState> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${fallback}: ${res.status}`),
    )
  }
  return (await res.json()) as OnboardingState
}

export async function fetchOnboardingState(): Promise<OnboardingState> {
  return readState(
    await fetch('/api/v1/onboarding'),
    'fetch onboarding state failed',
  )
}

export function onboardingQueryOptions() {
  return queryOptions({
    queryKey: onboardingKeys.all,
    queryFn: fetchOnboardingState,
    staleTime: 60_000,
  })
}

// Not suspense: the dashboard's empty state must still render if this fails.
export function useOnboardingStateOptional() {
  return useQuery({ ...onboardingQueryOptions(), retry: false })
}

export async function completeOnboarding(): Promise<OnboardingState> {
  return readState(
    await fetch('/api/v1/onboarding/complete', { method: 'POST' }),
    'complete onboarding failed',
  )
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

export async function updateOnboardingProgress(
  progress: OnboardingProgress,
): Promise<OnboardingState> {
  return readState(
    await fetch('/api/v1/onboarding/progress', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(progress),
    }),
    'save setup progress failed',
  )
}

export function useUpdateOnboardingProgress() {
  const queryClient = useQueryClient()
  return useMutation<OnboardingState, ApiError, OnboardingProgress>({
    mutationFn: updateOnboardingProgress,
    onSuccess: (data) => {
      queryClient.setQueryData(onboardingKeys.all, data)
    },
  })
}
