// Query-key factory and fetchers for /api/v1/notification-channels,
// mirroring queries/backupTargets.ts's own shared-queryOptions pattern.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  CreateNotificationChannelRequest,
  NotificationChannel,
  TestNotificationChannelRequest,
} from '../types/notificationChannel'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const notificationChannelKeys = {
  all: ['notification-channels'] as const,
  list: () => [...notificationChannelKeys.all, 'list'] as const,
}

export async function fetchNotificationChannels(): Promise<
  NotificationChannel[]
> {
  const res = await fetch('/api/v1/notification-channels')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch notification channels failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NotificationChannel[]
}

export function notificationChannelListQueryOptions() {
  return queryOptions({
    queryKey: notificationChannelKeys.list(),
    queryFn: fetchNotificationChannels,
  })
}

export function useNotificationChannels() {
  return useSuspenseQuery(notificationChannelListQueryOptions())
}

// Non-suspending counterpart for the per-app attach picker, which has no
// Suspense boundary: a failed list must degrade, not crash the page.
export function useNotificationChannelsOptional() {
  return useQuery({ ...notificationChannelListQueryOptions(), retry: false })
}

export async function createNotificationChannel(
  req: CreateNotificationChannelRequest,
): Promise<NotificationChannel> {
  const res = await fetch('/api/v1/notification-channels', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create notification channel failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NotificationChannel
}

export function useCreateNotificationChannel() {
  const queryClient = useQueryClient()
  return useMutation<
    NotificationChannel,
    ApiError,
    CreateNotificationChannelRequest
  >({
    mutationFn: createNotificationChannel,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: notificationChannelKeys.list(),
      })
    },
  })
}

export async function deleteNotificationChannel(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/notification-channels/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `delete notification channel failed: ${res.status}`,
    ),
  )
}

export function useDeleteNotificationChannel() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteNotificationChannel,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: notificationChannelKeys.list(),
      })
    },
  })
}

// POST /api/v1/notification-channels/test (handleTestNotificationChannel):
// tests kind+notify_url before the channel is ever saved, the connect
// dialog's "Send test message" action.
export async function testNotificationChannel(
  req: TestNotificationChannelRequest,
): Promise<void> {
  const res = await fetch('/api/v1/notification-channels/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `test notification failed: ${res.status}`),
  )
}

export function useTestNotificationChannel() {
  return useMutation<void, ApiError, TestNotificationChannelRequest>({
    mutationFn: testNotificationChannel,
  })
}

// POST /api/v1/notification-channels/{id}/test
// (handleTestExistingNotificationChannel): re-check an already-connected
// channel's own kind/notify_url.
export async function testExistingNotificationChannel(
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/notification-channels/${encodeURIComponent(id)}/test`,
    { method: 'POST' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `test notification failed: ${res.status}`),
  )
}

export function useTestExistingNotificationChannel() {
  return useMutation<void, ApiError, string>({
    mutationFn: testExistingNotificationChannel,
  })
}
