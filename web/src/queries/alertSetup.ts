// Alert templates (POST /api/v1/apps/{name}/alerts/apply-template) and
// channel quiet hours (/api/v1/notification-channels/{id}/quiet-hours).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { alertRuleKeys } from './alerts'
import type { AlertRule } from '../types/alerts'

export type AlertProfile = 'web' | 'worker' | 'database'

export interface ApplyAlertTemplateResult {
  profile: AlertProfile
  created: AlertRule[]
  skipped: string[]
}

export interface QuietHours {
  start: string
  end: string
  timezone: string
  enabled: boolean
}

const quietKey = (id: string) => ['channel-quiet-hours', id] as const

async function ensureOk(res: Response, fallback: string): Promise<void> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
}

export function useApplyAlertTemplate(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: {
      profile: AlertProfile
      channel_id?: string
    }): Promise<ApplyAlertTemplateResult> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/alerts/apply-template`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
      )
      await ensureOk(res, `apply alert template failed: ${res.status}`)
      return (await res.json()) as ApplyAlertTemplateResult
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: alertRuleKeys.list(appName),
      })
    },
  })
}

export function useChannelQuietHours(channelId: string) {
  return useQuery({
    queryKey: quietKey(channelId),
    queryFn: async (): Promise<QuietHours> => {
      const res = await fetch(
        `/api/v1/notification-channels/${encodeURIComponent(channelId)}/quiet-hours`,
      )
      await ensureOk(res, `load quiet hours failed: ${res.status}`)
      return (await res.json()) as QuietHours
    },
  })
}

export function useSaveChannelQuietHours(channelId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (q: QuietHours | null): Promise<void> => {
      const res = await fetch(
        `/api/v1/notification-channels/${encodeURIComponent(channelId)}/quiet-hours`,
        q
          ? {
              method: 'PUT',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(q),
            }
          : { method: 'DELETE' },
      )
      await ensureOk(res, `save quiet hours failed: ${res.status}`)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: quietKey(channelId) })
    },
  })
}
