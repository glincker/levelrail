import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import { appKeys } from './apps'
import { secretKeys } from './secrets'

export type StepUpAction = 'reveal' | 'export' | 'manage'

// Carries the server's machine-readable refusal code (step_up_required,
// step_up_invalid, reveal_disabled, app_write_only and so on).
export class EnvApiError extends ApiError {
  readonly code: string

  constructor(status: number, message: string, code: string) {
    super(status, message)
    this.name = 'EnvApiError'
    this.code = code
  }
}

export function isStepUpRequired(error: unknown): boolean {
  return error instanceof EnvApiError && error.code === 'step_up_required'
}

async function throwEnvError(res: Response, fallback: string): Promise<never> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    code?: string
  } | null
  throw new EnvApiError(res.status, body?.error ?? fallback, body?.code ?? '')
}

async function postJSON<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  })
  if (!res.ok) {
    return throwEnvError(res, `request failed: ${String(res.status)}`)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

const appPath = (app: string) => `/api/v1/apps/${encodeURIComponent(app)}`

export interface EnvSettings {
  reveal_policy: 'step_up' | 'disabled' | 'allow'
  policy_valid: boolean
  token_reveal: boolean
  secrets_write_only: boolean
  sensitive_overrides: Record<string, boolean>
  grant_ttl_seconds: number
  hide_after_seconds: number
  secrets_configured: boolean
}

export const envProtectionKeys = {
  settings: (app: string) => ['envProtection', 'settings', app] as const,
  stepUpOptions: ['envProtection', 'stepUpOptions'] as const,
}

export function useEnvSettings(app: string) {
  return useQuery({
    queryKey: envProtectionKeys.settings(app),
    queryFn: async (): Promise<EnvSettings> => {
      const res = await fetch(`${appPath(app)}/env/settings`)
      if (!res.ok) {
        return throwEnvError(
          res,
          `load env settings failed: ${String(res.status)}`,
        )
      }
      return (await res.json()) as EnvSettings
    },
  })
}

export interface StepUpOptions {
  methods: Array<'password' | 'passkey'>
  code_required: boolean
  policy: EnvSettings['reveal_policy']
  grant_ttl_seconds: number
  hide_after_seconds: number
}

export function useStepUpOptions(enabled: boolean) {
  return useQuery({
    queryKey: envProtectionKeys.stepUpOptions,
    enabled,
    queryFn: async (): Promise<StepUpOptions> => {
      const res = await fetch('/api/v1/auth/step-up')
      if (!res.ok) {
        return throwEnvError(
          res,
          `load step-up options failed: ${String(res.status)}`,
        )
      }
      return (await res.json()) as StepUpOptions
    },
  })
}

export interface StepUpScope {
  action: StepUpAction
  app: string
}

export function stepUpWithPassword(
  scope: StepUpScope,
  password: string,
  code: string,
): Promise<unknown> {
  return postJSON('/api/v1/auth/step-up', {
    method: 'password',
    password,
    code: code || undefined,
    action: scope.action,
    app: scope.app,
  })
}

interface PasskeyBegin {
  session_id: string
  options: { publicKey: PublicKeyCredentialRequestOptionsJSON }
}

export async function stepUpWithPasskey(scope: StepUpScope): Promise<unknown> {
  const begin = await postJSON<PasskeyBegin>(
    '/api/v1/auth/step-up/passkey/begin',
    {},
  )
  const options = PublicKeyCredential.parseRequestOptionsFromJSON(
    begin.options.publicKey,
  )
  const credential = await navigator.credentials.get({ publicKey: options })
  if (!(credential instanceof PublicKeyCredential)) {
    throw new EnvApiError(
      400,
      'The browser did not return a passkey.',
      'passkey_cancelled',
    )
  }
  const q = new URLSearchParams({
    session_id: begin.session_id,
    action: scope.action,
    app: scope.app,
  })
  return postJSON(
    `/api/v1/auth/step-up/passkey/finish?${q.toString()}`,
    credential.toJSON(),
  )
}

export interface RevealResult {
  key: string
  value: string
  type: 'plain' | 'secret'
  hide_after_seconds: number
}

export function revealValue(
  app: string,
  key: string,
  isSecret: boolean,
): Promise<RevealResult> {
  return isSecret
    ? postJSON(`${appPath(app)}/secrets/${encodeURIComponent(key)}/reveal`, {})
    : postJSON(`${appPath(app)}/env/reveal`, { key })
}

export type ExportFormat = 'dotenv' | 'json'

export async function exportEnv(
  app: string,
  format: ExportFormat,
  revealed: boolean,
): Promise<string> {
  const res = revealed
    ? await fetch(`${appPath(app)}/env/export`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ format }),
        cache: 'no-store',
      })
    : await fetch(`${appPath(app)}/env/export?format=${format}&values=masked`, {
        cache: 'no-store',
      })
  if (!res.ok) {
    return throwEnvError(res, `export failed: ${String(res.status)}`)
  }
  return res.text()
}

export interface EnvSettingsUpdate {
  sensitive_overrides?: Record<string, boolean | null>
  secrets_write_only?: boolean
}

export function useUpdateEnvSettings(app: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (body: EnvSettingsUpdate): Promise<EnvSettings> => {
      const res = await fetch(`${appPath(app)}/env/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        return throwEnvError(
          res,
          `save env settings failed: ${String(res.status)}`,
        )
      }
      return (await res.json()) as EnvSettings
    },
    onSuccess: (data) => {
      queryClient.setQueryData(envProtectionKeys.settings(app), data)
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(app) })
    },
  })
}

export async function deleteSecretValue(
  app: string,
  key: string,
): Promise<void> {
  const res = await fetch(
    `${appPath(app)}/secrets/${encodeURIComponent(key)}`,
    {
      method: 'DELETE',
    },
  )
  if (!res.ok && res.status !== 404) {
    await throwEnvError(res, `delete secret failed: ${String(res.status)}`)
  }
}

export function useConvertToSecret(app: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (key: string) =>
      postJSON<undefined>(
        `${appPath(app)}/env/${encodeURIComponent(key)}/convert-to-secret`,
        {},
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(app) })
      void queryClient.invalidateQueries({ queryKey: secretKeys.list(app) })
    },
  })
}
