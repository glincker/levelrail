// Query-key factory, fetcher, and replay mutation for
// GET /api/v1/apps/{name}/webhook-deliveries and
// POST .../webhook-deliveries/{id}/replay
// (internal/api/webhook_deliveries.go): recent inbound git provider
// webhook requests, and re-running one against its stored payload.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  ReplayWebhookDeliveryResult,
  WebhookDelivery,
} from '../types/webhookDelivery'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const webhookDeliveryKeys = {
  list: (appName: string) =>
    [...appKeys.detail(appName), 'webhook-deliveries'] as const,
}

export async function fetchWebhookDeliveries(
  appName: string,
): Promise<WebhookDelivery[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/webhook-deliveries`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch webhook deliveries failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as WebhookDelivery[] | null
  return body ?? []
}

export function useWebhookDeliveries(appName: string) {
  return useQuery({
    queryKey: webhookDeliveryKeys.list(appName),
    queryFn: () => fetchWebhookDeliveries(appName),
  })
}

export async function replayWebhookDelivery(
  appName: string,
  deliveryId: string,
): Promise<ReplayWebhookDeliveryResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/webhook-deliveries/${encodeURIComponent(deliveryId)}/replay`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `replay webhook delivery failed: ${res.status}`),
    )
  }
  return (await res.json()) as ReplayWebhookDeliveryResult
}

export function useReplayWebhookDelivery(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<ReplayWebhookDeliveryResult, ApiError, string>({
    mutationFn: (deliveryId) => replayWebhookDelivery(appName, deliveryId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: webhookDeliveryKeys.list(appName) })
    },
  })
}
