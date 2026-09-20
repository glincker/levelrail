// GET/PUT/DELETE /api/v1/settings/ai-assistant: the BYOK provider/model/
// API key configuration for the AI assistant chat feature. Follows the
// same single-row settings shape queries/emailSettings.ts and
// queries/cloudflareTunnel.ts already establish. api_key is write-only
// (PUT only, never read back); GET reports `configured` instead.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const aiAssistantSettingsKeys = {
  all: ['ai-assistant-settings'] as const,
}

export interface AiAssistantSettings {
  configured: boolean
  provider: string
  model: string
}

export interface UpdateAiAssistantSettingsRequest {
  provider: string
  model: string
  api_key: string
}

export async function fetchAiAssistantSettings(): Promise<AiAssistantSettings> {
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
  return (await res.json()) as AiAssistantSettings
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

// PUT's response shape isn't part of the fixed contract this was built
// against (only GET's is), so this refetches afterward instead of
// assuming the response body mirrors AiAssistantSettings.
export async function updateAiAssistantSettings(
  req: UpdateAiAssistantSettingsRequest,
): Promise<void> {
  const res = await fetch('/api/v1/settings/ai-assistant', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update AI assistant settings failed: ${res.status}`,
      ),
    )
  }
}

export function useUpdateAiAssistantSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateAiAssistantSettings,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: aiAssistantSettingsKeys.all,
      })
    },
  })
}

export async function clearAiAssistantSettings(): Promise<void> {
  const res = await fetch('/api/v1/settings/ai-assistant', {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `clear AI assistant settings failed: ${res.status}`,
      ),
    )
  }
}

export function useClearAiAssistantSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: clearAiAssistantSettings,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: aiAssistantSettingsKeys.all,
      })
    },
  })
}
