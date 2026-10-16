import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { aiAssistantSettingsKeys } from './aiAssistantSettings'

export type AiProviderKind = 'anthropic' | 'openai_compat' | 'gemini'

export type AiProviderStatus =
  'untested' | 'ok' | 'invalid_key' | 'unreachable' | 'quota_exhausted'

export interface AiProvider {
  id: string
  label: string
  kind: AiProviderKind
  default_base_url: string
  base_url_editable: boolean
  key_url: string
  default_model: string
  brand: string
  has_key: boolean
  base_url?: string
  status: AiProviderStatus
  status_detail?: string
  last_checked_at?: string
}

export interface AiSelection {
  provider: string
  model: string
}

export interface AiAssistantConfig {
  configured: boolean
  active?: AiSelection
  fallback: AiSelection[]
  limits: { max_steps?: number; monthly_token_budget?: number }
  token_usage_month?: number
}

export interface AiProvidersResult {
  providers: AiProvider[]
  legacy: boolean
}

export interface AiModel {
  id: string
  label: string
  context_window?: number
  supports_tools?: boolean
}

export interface AiUsageDay {
  date: string
  provider: string
  model: string
  prompt_tokens: number
  completion_tokens: number
  requests: number
  errors: number
}

export interface AiUsage {
  days: AiUsageDay[]
  totals?: Partial<
    Pick<AiUsageDay, 'prompt_tokens' | 'completion_tokens' | 'requests'>
  >
}

export interface AiTestResult {
  ok: boolean
  status: AiProviderStatus
  status_detail?: string
  latency_ms?: number
}

export interface AiProviderUpdate {
  api_key?: string
  base_url?: string
  default_model?: string
}

export interface AiProviderFailure {
  message: string
  code?: string
  fix?: string
}

export class AiProviderError extends Error {
  readonly status: number
  readonly code?: string
  readonly fix?: string

  constructor(status: number, failure: AiProviderFailure) {
    super(failure.message)
    this.name = 'AiProviderError'
    this.status = status
    this.code = failure.code
    this.fix = failure.fix
  }
}

const BASE = '/api/v1/settings/ai-assistant'

export const aiProviderKeys = {
  providers: ['ai-providers'] as const,
  config: ['ai-assistant-config'] as const,
  models: (id: string) => ['ai-provider-models', id] as const,
  usage: (days: number) => ['ai-usage', days] as const,
}

interface LegacySettings {
  configured?: boolean
  model?: string
}

export function legacyProviders(legacy: LegacySettings): AiProvider[] {
  return [
    {
      id: 'anthropic',
      label: 'Anthropic',
      kind: 'anthropic',
      default_base_url: '',
      base_url_editable: false,
      key_url: 'https://console.anthropic.com/settings/keys',
      default_model: legacy.model ?? '',
      brand: 'anthropic',
      has_key: Boolean(legacy.configured),
      status: legacy.configured ? 'ok' : 'untested',
    },
  ]
}

async function failureFrom(res: Response, fallback: string) {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    message?: string
    code?: string
    fix?: string
  } | null
  return new AiProviderError(res.status, {
    message: body?.error ?? body?.message ?? fallback,
    code: body?.code,
    fix: body?.fix,
  })
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export async function fetchAiProviders(): Promise<AiProvidersResult> {
  const res = await fetch(`${BASE}/providers`)
  if (res.ok) {
    const body = (await res.json().catch(() => null)) as AiProvider[] | null
    if (Array.isArray(body)) return { providers: body, legacy: false }
  }
  if (res.status === 404 || res.ok) {
    const legacy = await getJson<LegacySettings>(BASE, 'fetch AI settings')
    return { providers: legacyProviders(legacy), legacy: true }
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `fetch AI providers failed: ${res.status}`),
  )
}

export async function fetchAiConfig(): Promise<AiAssistantConfig> {
  const raw = await getJson<
    Partial<AiAssistantConfig> & { model?: string; provider?: string }
  >(BASE, 'fetch AI settings')
  const legacyActive =
    !raw.active && raw.configured && raw.model
      ? { provider: raw.provider || 'anthropic', model: raw.model }
      : undefined
  return {
    configured: Boolean(raw.configured),
    active: raw.active ?? legacyActive,
    fallback: raw.fallback ?? [],
    limits: raw.limits ?? {},
    token_usage_month: raw.token_usage_month,
  }
}

export function aiProvidersQueryOptions() {
  return queryOptions({
    queryKey: aiProviderKeys.providers,
    queryFn: fetchAiProviders,
    staleTime: 30_000,
  })
}

export function aiConfigQueryOptions() {
  return queryOptions({
    queryKey: aiProviderKeys.config,
    queryFn: fetchAiConfig,
    staleTime: 30_000,
  })
}

export function useAiProviders() {
  return useQuery(aiProvidersQueryOptions())
}

export function useAiConfig() {
  return useQuery(aiConfigQueryOptions())
}

export function useAiModels(id: string, enabled: boolean) {
  return useQuery({
    queryKey: aiProviderKeys.models(id),
    queryFn: () =>
      getJson<AiModel[]>(
        `${BASE}/providers/${encodeURIComponent(id)}/models`,
        'fetch models',
      ),
    enabled,
    staleTime: 5 * 60_000,
    retry: false,
  })
}

export function useAiUsage(days = 30) {
  return useQuery({
    queryKey: aiProviderKeys.usage(days),
    queryFn: () => getJson<AiUsage>(`/api/v1/ai/usage?days=${days}`, 'usage'),
    staleTime: 60_000,
    retry: false,
  })
}

function useRefreshAi() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: aiProviderKeys.providers })
    void queryClient.invalidateQueries({ queryKey: aiProviderKeys.config })
    void queryClient.invalidateQueries({
      queryKey: aiAssistantSettingsKeys.all,
    })
  }
}

async function sendJson(
  method: string,
  url: string,
  body: unknown,
  fallback: string,
): Promise<Response> {
  const res = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) throw await failureFrom(res, fallback)
  return res
}

export function useSaveAiProvider(legacy: boolean) {
  const refresh = useRefreshAi()
  return useMutation({
    mutationFn: async (vars: { id: string; update: AiProviderUpdate }) => {
      if (legacy) {
        await sendJson(
          'PUT',
          BASE,
          {
            model: vars.update.default_model ?? '',
            api_key: vars.update.api_key ?? '',
          },
          'save failed',
        )
        return
      }
      await sendJson(
        'PUT',
        `${BASE}/providers/${encodeURIComponent(vars.id)}`,
        vars.update,
        'save failed',
      )
    },
    onSuccess: refresh,
  })
}

export function useDisconnectAiProvider(legacy: boolean) {
  const refresh = useRefreshAi()
  return useMutation({
    mutationFn: async (id: string) => {
      const url = legacy ? BASE : `${BASE}/providers/${encodeURIComponent(id)}`
      await sendJson('DELETE', url, undefined, 'disconnect failed')
    },
    onSuccess: refresh,
  })
}

export function useTestAiProvider() {
  const refresh = useRefreshAi()
  return useMutation({
    mutationFn: async (id: string): Promise<AiTestResult> => {
      const res = await sendJson(
        'POST',
        `${BASE}/providers/${encodeURIComponent(id)}/test`,
        undefined,
        'test failed',
      )
      return (await res.json()) as AiTestResult
    },
    onSuccess: refresh,
  })
}

export function useUpdateAiRouting() {
  const refresh = useRefreshAi()
  return useMutation({
    mutationFn: async (body: {
      active?: AiSelection
      fallback?: AiSelection[]
    }) => {
      await sendJson('PUT', BASE, body, 'update failed')
    },
    onSuccess: refresh,
  })
}
