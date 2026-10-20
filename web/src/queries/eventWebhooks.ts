// Query keys, fetchers and mutations for /api/v1/event-webhooks
// (internal/api/event_webhooks.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CreateEventWebhookRequest,
  EventWebhook,
  EventWebhookDelivery,
  EventWebhookTestResult,
  UpdateEventWebhookRequest,
} from '../types/eventWebhooks'

export const eventWebhookKeys = {
  all: ['event-webhooks'] as const,
  list: () => [...eventWebhookKeys.all, 'list'] as const,
  deliveries: (id: string) =>
    [...eventWebhookKeys.all, 'deliveries', id] as const,
}

// Delivery counters change every few seconds, so both views are polled.
const REFETCH_MS = 10_000

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

const base = '/api/v1/event-webhooks'

export function useEventWebhooks() {
  return useQuery(
    queryOptions({
      queryKey: eventWebhookKeys.list(),
      queryFn: () =>
        request<EventWebhook[]>(base, undefined, 'fetch event webhooks'),
      refetchInterval: REFETCH_MS,
      retry: false,
    }),
  )
}

export function useEventWebhookDeliveries(id: string, enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: eventWebhookKeys.deliveries(id),
      queryFn: () =>
        request<EventWebhookDelivery[]>(
          `${base}/${encodeURIComponent(id)}/deliveries`,
          undefined,
          'fetch deliveries',
        ),
      refetchInterval: REFETCH_MS,
      enabled,
    }),
  )
}

function useInvalidatingMutation<TVars, TData>(
  fn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: eventWebhookKeys.all }),
  })
}

export function useCreateEventWebhook() {
  return useInvalidatingMutation((req: CreateEventWebhookRequest) =>
    request<EventWebhook>(base, jsonInit('POST', req), 'create event webhook'),
  )
}

export function useUpdateEventWebhook() {
  return useInvalidatingMutation(
    (vars: { id: string; body: UpdateEventWebhookRequest }) =>
      request<EventWebhook>(
        `${base}/${encodeURIComponent(vars.id)}`,
        jsonInit('PATCH', vars.body),
        'update event webhook',
      ),
  )
}

export function useDeleteEventWebhook() {
  return useInvalidatingMutation((id: string) =>
    request<void>(
      `${base}/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
      'delete event webhook',
    ),
  )
}

export function useTestEventWebhook() {
  return useInvalidatingMutation((id: string) =>
    request<EventWebhookTestResult>(
      `${base}/${encodeURIComponent(id)}/test`,
      { method: 'POST' },
      'test event webhook',
    ),
  )
}

export function useRotateEventWebhookSecret() {
  return useInvalidatingMutation((id: string) =>
    request<EventWebhook>(
      `${base}/${encodeURIComponent(id)}/rotate-secret`,
      { method: 'POST' },
      'rotate signing secret',
    ),
  )
}

export function useReplayEventDelivery() {
  return useInvalidatingMutation((deliveryId: string) =>
    request<EventWebhookDelivery>(
      `/api/v1/event-webhook-deliveries/${encodeURIComponent(deliveryId)}/replay`,
      { method: 'POST' },
      'replay delivery',
    ),
  )
}
