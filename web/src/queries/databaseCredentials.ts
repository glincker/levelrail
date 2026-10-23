import { EnvApiError } from './envProtection'

export const DB_CREDENTIALS_STEP_UP_APP = 'database-credentials'

export function credentialsStepUpScope(name: string) {
  return {
    action: 'manage',
    app: `${DB_CREDENTIALS_STEP_UP_APP}:${name}`,
  } as const
}

// Throws EnvApiError so useStepUpGate can catch step_up_required.
export async function revealDatabasePassword(name: string): Promise<string> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/credentials/reveal`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
      cache: 'no-store',
    },
  )
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new EnvApiError(
      res.status,
      body?.error ?? `reveal failed: ${res.status}`,
      body?.code ?? '',
    )
  }
  const data = (await res.json()) as { password: string }
  return data.password
}
