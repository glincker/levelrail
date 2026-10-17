// GET /api/v1/settings/ai-assistant: the active model, fallback chain and
// limits for the AI assistant. Provider keys never round-trip; `configured`
// says whether a usable provider chain exists.

import { queryOptions, useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const aiAssistantSettingsKeys = {
  all: ['ai-assistant-settings'] as const,
}

export interface AiSelection {
  provider: string
  model: string
}

export interface AiAssistantConfig {
  configured: boolean
  active?: AiSelection
  fallback: AiSelection[]
  limits: { max_steps: number; monthly_token_budget?: number }
  token_usage_month?: number
}

export async function fetchAiAssistantSettings(): Promise<AiAssistantConfig> {
  const res = await fetch('/api/v1/settings/ai-assistant')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch AI assistant settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AiAssistantConfig
}

export function aiAssistantSettingsQueryOptions() {
  return queryOptions({
    queryKey: aiAssistantSettingsKeys.all,
    queryFn: fetchAiAssistantSettings,
    staleTime: 60_000,
  })
}

export function useAiAssistantSettings() {
  return useSuspenseQuery(aiAssistantSettingsQueryOptions())
}

export function useAiConfig() {
  return useQuery(aiAssistantSettingsQueryOptions())
}
