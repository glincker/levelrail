// Query-key factory and fetchers for /api/v1/settings/push-subscriptions,
// mirroring queries/notificationChannels.ts's own plain-fetch pattern.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  CreatePushSubscriptionRequest,
  PushSubscription,
  PushVAPIDPublicKeyResponse,
} from '../types/pushSubscription'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const pushSubscriptionKeys = {
  all: ['push-subscriptions'] as const,
  list: () => [...pushSubscriptionKeys.all, 'list'] as const,
  vapidPublicKey: () =>
    [...pushSubscriptionKeys.all, 'vapid-public-key'] as const,
}

export async function fetchPushVAPIDPublicKey(): Promise<string> {
  const res = await fetch(
    '/api/v1/settings/push-subscriptions/vapid-public-key',
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch push vapid public key failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as PushVAPIDPublicKeyResponse
  return body.public_key
}

// retry: false, since a 501 (no master key configured) is an expected,
// permanent "feature not available" result, not a transient failure
// worth React Query's default retries.
export function usePushVAPIDPublicKey() {
  return useQuery({
    queryKey: pushSubscriptionKeys.vapidPublicKey(),
    queryFn: fetchPushVAPIDPublicKey,
    retry: false,
  })
}

export async function fetchPushSubscriptions(): Promise<PushSubscription[]> {
  const res = await fetch('/api/v1/settings/push-subscriptions')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch push subscriptions failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as PushSubscription[]
}

export function pushSubscriptionListQueryOptions() {
  return queryOptions({
    queryKey: pushSubscriptionKeys.list(),
    queryFn: fetchPushSubscriptions,
  })
}

export function usePushSubscriptions(enabled = true) {
  return useQuery({
    ...pushSubscriptionListQueryOptions(),
    enabled,
    retry: false,
  })
}

export async function createPushSubscription(
  req: CreatePushSubscriptionRequest,
): Promise<PushSubscription> {
  const res = await fetch('/api/v1/settings/push-subscriptions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `register push subscription failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as PushSubscription
}

export function useCreatePushSubscription() {
  const queryClient = useQueryClient()
  return useMutation<PushSubscription, ApiError, CreatePushSubscriptionRequest>(
    {
      mutationFn: createPushSubscription,
      onSuccess: () => {
        void queryClient.invalidateQueries({
          queryKey: pushSubscriptionKeys.list(),
        })
      },
    },
  )
}

export async function deletePushSubscription(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/settings/push-subscriptions/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `revoke push subscription failed: ${res.status}`,
    ),
  )
}

export function useDeletePushSubscription() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deletePushSubscription,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: pushSubscriptionKeys.list(),
      })
    },
  })
}
