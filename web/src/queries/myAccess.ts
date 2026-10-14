import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { sessionKeys } from './security'

// Own-account endpoints: internal/api/account_access.go.

export interface AccessTeam {
  id: string
  name: string
  role: string
}

export interface AccessPolicy {
  id: string
  name: string
  description: string
}

export interface AccessCapability {
  key: string
  allowed: boolean
  required_ability: string
  reason?: string
}

export interface MyAccess {
  kind: string
  name: string
  tier: string
  abilities: string[]
  teams: AccessTeam[]
  linked_providers: string[]
  policies: AccessPolicy[]
  capabilities: AccessCapability[]
}

export const accountKeys = {
  access: ['account', 'my-access'] as const,
}

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function myAccessQueryOptions() {
  return queryOptions({
    queryKey: accountKeys.access,
    queryFn: () => send<MyAccess>('GET', '/api/v1/auth/my-access'),
  })
}

export function useUpdateDisplayName() {
  const qc = useQueryClient()
  return useMutation<{ display_name: string }, ApiError, string>({
    mutationFn: (displayName) =>
      send('PATCH', '/api/v1/auth/profile', { display_name: displayName }),
    onSuccess: () => qc.invalidateQueries({ queryKey: sessionKeys.all }),
  })
}
